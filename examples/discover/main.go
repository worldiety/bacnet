package main

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"time"

	"go.wdy.de/bacnet"
	"go.wdy.de/bacnet/apdu"
	"go.wdy.de/bacnet/common/netprim"
)

func main() {
	err := runDiscover()
	if err != nil {
		log.Fatal("discover failed", err)
	}
}

func runDiscover() error {
	rt, err := bacnet.NewClientRuntime(netip.AddrFrom4([4]byte{0, 0, 0, 0}), bacnet.ClientRuntimeConfig{
		MaxDatagramSize: 0,
		ASE:             apdu.DefaultASEConfig(),
		Client:          apdu.DefaultClientConfig(),
	})
	if err != nil {
		return err
	}
	defer rt.Close()

	go func() {
		_ = rt.Run(context.Background())
	}()

	devices, err := rt.Client().Discover(context.Background(), apdu.DiscoverRequest{
		Destination: netprim.Address{
			Network:  netprim.LocalNetwork,
			AddrPort: netip.AddrPortFrom(netip.MustParseAddr("192.168.8.255"), 47808),
		},
		WhoIs:  apdu.NewWhoIsRequest(),
		Window: time.Second * 20,
	})
	if err != nil {
		return err
	}

	for _, device := range devices {
		fmt.Println(device)
	}

	return nil
}
