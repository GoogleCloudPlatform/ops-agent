package confgenerator

import "context"

type globalConfigKey struct{}

func ContextWithGlobalConfig(ctx context.Context, global *Global) context.Context {
	return context.WithValue(ctx, globalConfigKey{}, global)
}

func GlobalConfigFromContext(ctx context.Context) *Global {
	global, _ := ctx.Value(globalConfigKey{}).(*Global)
	return global
}
