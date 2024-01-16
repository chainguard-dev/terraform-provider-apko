package main

import (
	"context"
	"flag"
	"log"
	"log/slog"

	"github.com/chainguard-dev/terraform-provider-apko/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

//go:generate terraform fmt -recursive ./examples/
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

const version string = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/chainguard-dev/apko",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}

func init() {
	slog.SetDefault(slog.New(tfhandler{}))
}

type tfhandler struct{ attrs []slog.Attr }

const subsystem = "apko"

func (h tfhandler) Handle(ctx context.Context, r slog.Record) error {
	// This is a bit of a hack, but it's the only way to get the correct
	// source location for the log message.
	//
	// This creates a new tflog subsystem for logging, with the location
	// offset set to 3, which is the number of frames between this function
	// and the actual logging call site. Then we use this subsystem below to log
	// the message to TF's logger.
	ctx = tflog.NewSubsystem(ctx, subsystem, tflog.WithAdditionalLocationOffset(3))

	addl := make(map[string]interface{})
	r.Attrs(func(s slog.Attr) bool {
		addl[s.Key] = s.Value.String()
		return true
	})
	for _, a := range h.attrs {
		addl[a.Key] = a.Value.String()
	}

	switch r.Level {
	case slog.LevelDebug:
		tflog.SubsystemDebug(ctx, subsystem, r.Message, addl)
	case slog.LevelInfo:
		tflog.SubsystemInfo(ctx, subsystem, r.Message, addl)
	case slog.LevelWarn:
		tflog.SubsystemWarn(ctx, subsystem, r.Message, addl)
	case slog.LevelError:
		tflog.SubsystemError(ctx, subsystem, r.Message, addl)
	default:
		tflog.SubsystemInfo(ctx, subsystem, r.Message, addl)
	}
	return nil
}

func (_ tfhandler) Enabled(context.Context, slog.Level) bool { return true }
func (h tfhandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return tfhandler{attrs: append(h.attrs, attrs...)}
}
func (_ tfhandler) WithGroup(name string) slog.Handler { panic("unimplemented") }
