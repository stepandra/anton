package main

import (
	"os"

	"github.com/allisson/go-env"
	"github.com/urfave/cli/v2"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/stepandra/anton/cmd/archive"
	"github.com/stepandra/anton/cmd/contract"
	"github.com/stepandra/anton/cmd/db"
	"github.com/stepandra/anton/cmd/indexer"
	"github.com/stepandra/anton/cmd/label"
	"github.com/stepandra/anton/cmd/rescan"
	"github.com/stepandra/anton/cmd/web"
)

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	level := zerolog.InfoLevel
	if env.GetBool("DEBUG_LOGS", false) {
		level = zerolog.DebugLevel
	}

	// add file and line number to log
	log.Logger = log.With().Caller().Logger().Level(level)
}

func main() {
	app := &cli.App{
		Name:  "anton",
		Usage: "an indexing project",
		Commands: []*cli.Command{
			db.Command,
			indexer.Command,
			web.Command,
			archive.Command,
			contract.Command,
			label.Command,
			rescan.Command,
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal().Err(err).Msg("")
	}
}
