package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	stdrpc "net/rpc"
	"os"
	"time"

	rpcadapter "fx-settlement-lab/go-backend/internal/adapter/inbound/rpc"
	fxv1 "fx-settlement-lab/go-backend/proto/fx/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	defaultHTTPBaseURL = "http://127.0.0.1:8000"
	defaultGRPCAddr    = "127.0.0.1:9000"
	defaultRPCAddr     = "127.0.0.1:9100"
	defaultTimeout     = 30 * time.Second
	requestTimeout     = 5 * time.Second
)

type config struct {
	httpBaseURL string
	grpcAddr    string
	rpcAddr     string
	timeout     time.Duration
}

type httpQuoteEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		ID string `json:"id"`
	} `json:"data"`
}

type httpConversionEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		ID string `json:"id"`
	} `json:"data"`
}

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	cfg := config{
		httpBaseURL: envOrDefault("DEMO_HTTP_BASE_URL", defaultHTTPBaseURL),
		grpcAddr:    envOrDefault("DEMO_GRPC_ADDR", defaultGRPCAddr),
		rpcAddr:     envOrDefault("DEMO_RPC_ADDR", defaultRPCAddr),
		timeout:     defaultTimeout,
	}

	if err := waitForHTTPReady(cfg.httpBaseURL, cfg.timeout); err != nil {
		return err
	}
	if err := waitForGRPCReady(cfg.grpcAddr, cfg.timeout); err != nil {
		return err
	}
	if err := waitForRPCReady(cfg.rpcAddr, cfg.timeout); err != nil {
		return err
	}

	if err := driveHTTP(cfg); err != nil {
		return err
	}
	if err := driveGRPC(cfg); err != nil {
		return err
	}
	if err := driveRPC(cfg); err != nil {
		return err
	}

	fmt.Println("demo traffic completed for http, grpc, rpc")
	return nil
}

func driveHTTP(cfg config) error {
	client := &http.Client{Timeout: requestTimeout}

	if err := expectHTTPStatus(client, http.MethodGet, cfg.httpBaseURL+"/api/v1/rates?base=KRW&quotes=USD,JPY", nil, nil, http.StatusOK); err != nil {
		return fmt.Errorf("http get rates: %w", err)
	}

	quoteID, err := createHTTPQuote(client, cfg.httpBaseURL, nextKey("http-quote"))
	if err != nil {
		return fmt.Errorf("http create quote: %w", err)
	}

	conversionID, err := createHTTPConversion(client, cfg.httpBaseURL, quoteID, nextKey("http-conversion"))
	if err != nil {
		return fmt.Errorf("http create conversion: %w", err)
	}

	if err := expectHTTPStatus(client, http.MethodGet, cfg.httpBaseURL+"/api/v1/conversions/"+conversionID, nil, nil, http.StatusOK); err != nil {
		return fmt.Errorf("http get conversion: %w", err)
	}

	if err := expectHTTPStatus(client, http.MethodPost, cfg.httpBaseURL+"/api/v1/quotes", map[string]any{
		"baseAmount": map[string]any{
			"currency":   "KRW",
			"minorUnits": 100000,
		},
		"quoteCurrency": "USD",
	}, map[string]string{"Content-Type": "application/json"}, http.StatusUnprocessableEntity); err != nil {
		return fmt.Errorf("http missing idempotency: %w", err)
	}

	return nil
}

func driveGRPC(cfg config) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, cfg.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("dial grpc: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := fxv1.NewFXServiceClient(conn)

	callCtx, callCancel := context.WithTimeout(context.Background(), requestTimeout)
	_, err = client.GetRates(callCtx, &fxv1.GetRatesRequest{Base: "KRW", Quotes: []string{"USD", "JPY"}})
	callCancel()
	if err != nil {
		return fmt.Errorf("grpc get rates: %w", err)
	}

	quoteCtx, quoteCancel := context.WithTimeout(context.Background(), requestTimeout)
	quote, err := client.CreateQuote(
		metadata.AppendToOutgoingContext(quoteCtx, "idempotency-key", nextKey("grpc-quote")),
		&fxv1.CreateQuoteRequest{
			BaseAmount:    &fxv1.MoneyInput{Currency: "KRW", MinorUnits: 100000},
			QuoteCurrency: "USD",
		},
	)
	quoteCancel()
	if err != nil {
		return fmt.Errorf("grpc create quote: %w", err)
	}

	conversionCtx, conversionCancel := context.WithTimeout(context.Background(), requestTimeout)
	conversion, err := client.CreateConversion(
		metadata.AppendToOutgoingContext(conversionCtx, "idempotency-key", nextKey("grpc-conversion")),
		&fxv1.CreateConversionRequest{QuoteId: quote.GetId()},
	)
	conversionCancel()
	if err != nil {
		return fmt.Errorf("grpc create conversion: %w", err)
	}

	getCtx, getCancel := context.WithTimeout(context.Background(), requestTimeout)
	_, err = client.GetConversion(getCtx, &fxv1.GetConversionRequest{ConversionId: conversion.GetId()})
	getCancel()
	if err != nil {
		return fmt.Errorf("grpc get conversion: %w", err)
	}

	errorCtx, errorCancel := context.WithTimeout(context.Background(), requestTimeout)
	_, err = client.CreateQuote(errorCtx, &fxv1.CreateQuoteRequest{
		BaseAmount:    &fxv1.MoneyInput{Currency: "KRW", MinorUnits: 100000},
		QuoteCurrency: "USD",
	})
	errorCancel()
	if status.Code(err) != codes.InvalidArgument {
		return fmt.Errorf("grpc missing idempotency expected InvalidArgument, got %v", err)
	}

	return nil
}

func driveRPC(cfg config) error {
	client, err := stdrpc.Dial("tcp", cfg.rpcAddr)
	if err != nil {
		return fmt.Errorf("dial rpc: %w", err)
	}
	defer func() { _ = client.Close() }()

	var ratesReply rpcadapter.GetRatesReply
	if err := client.Call("FXRPCService.GetRates", rpcadapter.GetRatesArgs{
		Base:   "KRW",
		Quotes: []string{"USD", "JPY"},
	}, &ratesReply); err != nil {
		return fmt.Errorf("rpc get rates: %w", err)
	}

	var quoteReply rpcadapter.QuoteReply
	if err := client.Call("FXRPCService.CreateQuote", rpcadapter.CreateQuoteArgs{
		IdempotencyKey: nextKey("rpc-quote"),
		BaseAmount:     rpcadapter.MoneyInput{Currency: "KRW", MinorUnits: 100000},
		QuoteCurrency:  "USD",
	}, &quoteReply); err != nil {
		return fmt.Errorf("rpc create quote: %w", err)
	}

	var conversionReply rpcadapter.ConversionReply
	if err := client.Call("FXRPCService.CreateConversion", rpcadapter.CreateConversionArgs{
		IdempotencyKey: nextKey("rpc-conversion"),
		QuoteID:        quoteReply.ID,
	}, &conversionReply); err != nil {
		return fmt.Errorf("rpc create conversion: %w", err)
	}

	var loadedConversion rpcadapter.ConversionReply
	if err := client.Call("FXRPCService.GetConversion", rpcadapter.GetConversionArgs{ConversionID: conversionReply.ID}, &loadedConversion); err != nil {
		return fmt.Errorf("rpc get conversion: %w", err)
	}

	var errorReply rpcadapter.QuoteReply
	if err := client.Call("FXRPCService.CreateQuote", rpcadapter.CreateQuoteArgs{
		BaseAmount:    rpcadapter.MoneyInput{Currency: "KRW", MinorUnits: 100000},
		QuoteCurrency: "USD",
	}, &errorReply); err == nil {
		return fmt.Errorf("rpc missing idempotency expected error, got nil")
	}

	return nil
}

func createHTTPQuote(client *http.Client, baseURL string, idempotencyKey string) (string, error) {
	body := map[string]any{
		"baseAmount": map[string]any{
			"currency":   "KRW",
			"minorUnits": 100000,
		},
		"quoteCurrency": "USD",
	}

	responseBody, err := doJSONRequest(client, http.MethodPost, baseURL+"/api/v1/quotes", body, map[string]string{
		"Content-Type":    "application/json",
		"Idempotency-Key": idempotencyKey,
	}, http.StatusOK)
	if err != nil {
		return "", err
	}

	var envelope httpQuoteEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return "", fmt.Errorf("decode http quote response: %w", err)
	}

	return envelope.Data.ID, nil
}

func createHTTPConversion(client *http.Client, baseURL string, quoteID string, idempotencyKey string) (string, error) {
	body := map[string]any{"quoteId": quoteID}

	responseBody, err := doJSONRequest(client, http.MethodPost, baseURL+"/api/v1/conversions", body, map[string]string{
		"Content-Type":    "application/json",
		"Idempotency-Key": idempotencyKey,
	}, http.StatusOK)
	if err != nil {
		return "", err
	}

	var envelope httpConversionEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return "", fmt.Errorf("decode http conversion response: %w", err)
	}

	return envelope.Data.ID, nil
}

func expectHTTPStatus(client *http.Client, method string, url string, body any, headers map[string]string, expectedStatus int) error {
	_, err := doJSONRequest(client, method, url, body, headers, expectedStatus)
	return err
}

func doJSONRequest(client *http.Client, method string, url string, body any, headers map[string]string, expectedStatus int) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if response.StatusCode != expectedStatus {
		return nil, fmt.Errorf("unexpected status %d body=%s", response.StatusCode, string(responseBody))
	}

	return responseBody, nil
}

func waitForHTTPReady(baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: requestTimeout}

	return waitFor("http", timeout, func() error {
		response, err := client.Get(baseURL + "/ready")
		if err != nil {
			return err
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("ready status %d", response.StatusCode)
		}

		return nil
	})
}

func waitForGRPCReady(addr string, timeout time.Duration) error {
	return waitFor("grpc", timeout, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err != nil {
			return err
		}
		_ = conn.Close()
		return nil
	})
}

func waitForRPCReady(addr string, timeout time.Duration) error {
	return waitFor("rpc", timeout, func() error {
		client, err := stdrpc.Dial("tcp", addr)
		if err != nil {
			return err
		}
		_ = client.Close()
		return nil
	})
}

func waitFor(name string, timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("wait for %s ready: %w", name, lastErr)
}

func nextKey(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
