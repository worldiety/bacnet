package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/netip"
	"os"
	"time"

	"github.com/worldiety/bacnet"
	"github.com/worldiety/bacnet/apdu"
	baclog "github.com/worldiety/bacnet/common/log"
	"github.com/worldiety/bacnet/common/netprim"
)

func main() {
	args := os.Args[1:]
	err := runDiscover(args)
	if err != nil {
		log.Fatal("discover failed", err)
	}
}

func runDiscover(args []string) error {
	baclog.InitLogger(slog.LevelDebug, false)

	rt, err := bacnet.NewClientRuntime(netip.AddrFrom4([4]byte{0, 0, 0, 0}), bacnet.ClientRuntimeConfig{
		MaxDatagramSize: 0,
		ASE:             apdu.DefaultASEConfig(),
		Client:          apdu.DefaultClientConfig(),
	})
	if err != nil {
		return err
	}

	go func() {
		_ = rt.Run(context.Background())
	}()

	devices, err := rt.Client().Discover(context.Background(), apdu.DiscoverRequest{
		Destination: netprim.Address{
			Network:  netprim.LocalNetwork,
			AddrPort: netip.AddrPortFrom(netip.MustParseAddr(args[0]), 47808),
		},
		WhoIs:  apdu.NewWhoIsRequest(),
		Window: time.Second * 10,
	})
	if err != nil {
		return err
	}

	for _, device := range devices {
		fmt.Println(device)
	}

	if len(devices) == 0 {
		fmt.Println("No devices")
	}

	return nil
}
