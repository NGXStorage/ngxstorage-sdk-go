// Command ngxstorage is a CLI for the NGX Storage Manager API v2, built on
// the ngxstorage Go SDK. It demonstrates the SDK and provides a lightweight
// operational tool for listing pools/LUNs/targets and querying cluster status.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	ngxstorage "github.com/ngxstorage/ngxstorage-sdk-go/pkg/ngxstorage"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ngxstorage:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var (
		controllers = flag.String("controllers", "", "comma-separated controller IPs (required)")
		apiKey      = flag.String("api-key", "", "NGX API key (required)")
		poolName    = flag.String("pool-name", "", "canonical pool name")
		skipVerify  = flag.Bool("insecure-skip-verify", true, "skip TLS verification (self-signed NGX appliances)")
	)
	flag.CommandLine.Parse(args)
	rest := flag.Args()

	if *controllers == "" || *apiKey == "" {
		return fmt.Errorf("--controllers and --api-key are required")
	}

	client, err := ngxstorage.NewClient(ngxstorage.Config{
		Controllers:        strings.Split(*controllers, ","),
		APIKey:             *apiKey,
		PoolName:           *poolName,
		InsecureSkipVerify: *skipVerify,
		Logger:             ngxstorage.NopLogger{},
	})
	if err != nil {
		return err
	}

	ctx := context.Background()
	if len(rest) == 0 {
		printUsage()
		return nil
	}

	switch rest[0] {
	case "status":
		cluster, err := client.Status().Cluster(ctx)
		if err != nil {
			return err
		}
		return printJSON(cluster)
	case "pools":
		pools, err := client.Pools().List(ctx)
		if err != nil {
			return err
		}
		return printJSON(pools)
	case "luns":
		luns, err := client.LUNs().List(ctx)
		if err != nil {
			return err
		}
		return printJSON(luns)
	case "shares":
		shares, err := client.Shares().List(ctx)
		if err != nil {
			return err
		}
		return printJSON(shares)
	case "fctargets":
		targets, err := client.FCTargets().List(ctx)
		if err != nil {
			return err
		}
		return printJSON(targets)
	case "snapshots":
		snaps, err := client.Snapshots().List(ctx)
		if err != nil {
			return err
		}
		return printJSON(snaps)
	case "create-lun":
		if len(rest) < 3 {
			return fmt.Errorf("usage: create-lun <name> <size-gb>")
		}
		var sizeGB int64
		fmt.Sscanf(rest[2], "%d", &sizeGB)
		lun, err := client.LUNs().Create(ctx, rest[1], sizeGB, "1")
		if err != nil {
			return err
		}
		return printJSON(lun)
	case "delete-lun":
		if len(rest) < 2 {
			return fmt.Errorf("usage: delete-lun <id>")
		}
		return client.LUNs().Delete(ctx, rest[1])
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `ngxstorage — NGX Storage Manager API v2 CLI

usage: ngxstorage --controllers <ips> --api-key <key> <command> [args]

commands:
  status             print cluster work-mode status
  pools              list storage pools
  luns               list block volumes (LUNs)
  shares             list file volumes (shares)
  fctargets          list Fibre Channel targets
  snapshots          list snapshots
  create-lun <name> <size-gb>   create a LUN
  delete-lun <id>               delete a LUN`)
}
