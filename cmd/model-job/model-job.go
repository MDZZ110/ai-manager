package main

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var Logger = CreateLogger()

func main() {
	cmd := NewModelJobCommand()
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}

func NewModelJobCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "runModelJob",
		Long: `Run Assemble Model Image Job`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunAssembleModelImageJob()
		},
	}

	return cmd
}

func RunAssembleModelImageJob() (err error) {
	// TODO load config file

	// TODO pull model file

	// TODO split model file to layers

	// TODO write model files to Tar

	// TODO rebase image

	// TODO push image to registry

	// TODO consider OS arch
}

func CreateLogger() *zap.Logger {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	config := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: false,
		Sampling:          nil,
		Encoding:          "json",
		EncoderConfig:     encoderCfg,
		OutputPaths: []string{
			"stderr",
		},
		ErrorOutputPaths: []string{
			"stderr",
		},
		InitialFields: map[string]interface{}{
			"pid": os.Getpid(),
		},
	}

	return zap.Must(config.Build())
}
