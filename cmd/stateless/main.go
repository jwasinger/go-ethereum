package main

import (
	"bufio"
	"fmt"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/console/prompt"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/internal/flags"
	"github.com/urfave/cli/v2"
	"go.uber.org/automaxprocs/maxprocs"
	"os"
)

var (
	BlockWitnessFlag = &cli.StringFlag{
		Name:  "block-witness",
		Usage: "foo bar",
	}

	BlockWitness1Flag = &cli.StringFlag{
		Name:  "witness1",
		Usage: "foo bar",
	}
	BlockWitness2Flag = &cli.StringFlag{
		Name:  "witness2",
		Usage: "foo bar",
	}

	LogFileFlag = &cli.StringFlag{
		Name:  "logfile",
		Usage: "if present, generate debug trace (just evm traces in the future).  store trace to the file",
	}

	WitnessDiffCommand = &cli.Command{
		Action:    witnessDiff,
		Name:      "diff",
		Usage:     "",
		ArgsUsage: "<genesisPath>",
		Flags: []cli.Flag{
			BlockWitness1Flag,
			BlockWitness2Flag,
		},
		Description: `placeholder description`,
	}
	PPCommand = &cli.Command{
		Action:    pp,
		Name:      "pp",
		Usage:     "",
		ArgsUsage: "<genesisPath>",
		Flags: []cli.Flag{
			BlockWitnessFlag,
		},
		Description: `placeholder description`,
	}
	StatelessCommand = &cli.Command{
		Action:    statelessCmd,
		Name:      "exec",
		Usage:     "",
		ArgsUsage: "<genesisPath>",
		Flags: []cli.Flag{
			BlockWitnessFlag,
			LogFileFlag,
		},
		Description: `placeholder description`,
	}
	ServerCommand = &cli.Command{
		Action:      server,
		Name:        "server",
		Usage:       "",
		ArgsUsage:   "<genesisPath>",
		Flags:       []cli.Flag{},
		Description: `placeholder description`,
	}
)

var app = flags.NewApp("stateless block executor")

func init() {
	// Initialize the CLI app and start Geth
	app.Copyright = "Copyright 2013-2023 The go-ethereum Authors"
	app.Commands = []*cli.Command{
		WitnessDiffCommand,
		PPCommand,
		StatelessCommand,
		ServerCommand,
	}

	app.Flags = []cli.Flag{
		BlockWitnessFlag,
	}

	app.Before = func(ctx *cli.Context) error {
		maxprocs.Set() // Automatically set GOMAXPROCS to match Linux container CPU quota.
		if err := debug.Setup(ctx); err != nil {
			return err
		}
		return nil
	}
	app.After = func(ctx *cli.Context) error {
		debug.Exit()
		prompt.Stdin.Close() // Resets terminal mode.
		return nil
	}
}

func statelessCmd(ctx *cli.Context) error {
	var logWriter *bufio.Writer
	blockWitnessPath := ctx.String(BlockWitnessFlag.Name)
	if blockWitnessPath == "" {
		panic("block witness required")
	}

	logFile := ctx.String(LogFileFlag.Name)
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY, 0744)
		if err != nil {
			return err
		}
		logWriter = bufio.NewWriter(f)
		if err != nil {
			panic(err)
		}
		defer logWriter.Flush()
	}

	b, err := os.ReadFile(blockWitnessPath)
	if err != nil {
		panic(err)
	}

	witness, err := state.DecodeWitnessRLP(b)
	if err != nil {
		panic(err)
	}

	fmt.Println(witness.Block.GasUsed())
	fmt.Printf("%+v\n", witness.Block.Header())

	correct, err := utils.StatelessVerify(logWriter, witness)
	if err != nil {
		panic(err)
	}
	if !correct {
		panic("not correct")
	}
	return nil
}

func pp(ctx *cli.Context) error {
	witnessPath := ctx.String(BlockWitnessFlag.Name)
	b, err := os.ReadFile(witnessPath)
	if err != nil {
		return err
	}
	w, err := state.DecodeWitnessRLP(b)
	if err != nil {
		panic(err)
	}

	fmt.Println(w.PrettyPrint())
	return nil
}

func witnessDiff(ctx *cli.Context) error {
	witness1Path := ctx.String(BlockWitness1Flag.Name)
	witness2Path := ctx.String(BlockWitness2Flag.Name)

	b1, err := os.ReadFile(witness1Path)
	if err != nil {
		return err
	}

	b2, err := os.ReadFile(witness2Path)
	if err != nil {
		return err
	}

	w1, err := state.DecodeWitnessRLP(b1)
	if err != nil {
		panic(err)
	}

	w2, err := state.DecodeWitnessRLP(b2)
	if err != nil {
		panic(err)
	}

	w1Hash := w1.Hash()
	w2Hash := w2.Hash()
	if w1Hash != w2Hash {
		fmt.Printf("witness 1 hash (%x) != witness 2 hash (%x)\n", w1Hash, w2Hash)
	}
	return nil
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
