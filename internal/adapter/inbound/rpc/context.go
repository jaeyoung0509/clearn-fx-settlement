package rpcadapter

import "context"

func nilContext() context.Context {
	return context.Background()
}
