package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/netip"
	"os"
	"time"

	"go.wdy.de/bacnet"
	"go.wdy.de/bacnet/apdu"
	baclog "go.wdy.de/bacnet/common/log"
	"go.wdy.de/bacnet/common/netprim"
	"go.wdy.de/bacnet/common/types"
	"go.wdy.de/bacnet/encoding"
)

func main() {
	args := os.Args[1:]

	err := runRead(args)
	if err != nil {
		log.Fatal(err)
	}
}

func runRead(args []string) error {
	baclog.InitLogger(slog.LevelDebug, true)

	addrStr := args[0]
	broadcastAddrStr := args[1]

	addr, err := netip.ParseAddrPort(addrStr)
	if err != nil {
		return err
	}

	broadcastAddr, err := netip.ParseAddrPort(broadcastAddrStr)
	if err != nil {
		return err
	}

	log.Printf("Read from %s\n", addr)

	rt, err := bacnet.NewClientRuntime(netip.AddrFrom4([4]byte{0, 0, 0, 0}), bacnet.DefaultClientRuntimeConfig())
	if err != nil {
		return err
	}

	go func() {
		_ = rt.Run(context.Background())
	}()

	_, err = rt.Client().Discover(context.Background(), apdu.DiscoverRequest{
		Destination: netprim.NewAddressFromAddrPort(broadcastAddr),
		WhoIs:       apdu.NewWhoIsRequest(),
		Window:      time.Second * 5,
	})
	if err != nil {
		log.Printf("device not reachable")
		return err
	}

	analogInput1, _ := types.NewObjectIdentifier(types.ObjectTypeAnalogInput, 250)
	device1234, _ := types.NewObjectIdentifier(types.ObjectTypeDevice, 1234)

	req, err := apdu.NewReadPropertyMultipleRequest([]apdu.ReadAccessSpecification{
		{
			ObjectIdentifier: analogInput1,
			Properties: []apdu.PropertyReference{
				{PropertyIdentifier: types.PropertyIdentifierObjectName},
				{PropertyIdentifier: types.PropertyIdentifierPresentValue},
			},
		},
		{
			ObjectIdentifier: device1234,
			Properties: []apdu.PropertyReference{
				{PropertyIdentifier: types.PropertyIdentifierObjectName},
				{PropertyIdentifier: types.PropertyIdentifierPresentValue},
			},
		},
	})

	ack, err := rt.Client().ReadPropertyMultiple(context.Background(), netprim.NewAddressFromAddrPort(addr), req)
	if err != nil {
		var remoteErr apdu.RemoteErrorAPDU
		switch {
		case errors.As(err, &remoteErr):
			fmt.Printf("device refused: %s/%s\n", remoteErr.ErrorClass, remoteErr.ErrorCode)
		case errors.Is(err, apdu.ErrAPDUTimeout):
			fmt.Println("device did not respond within the invoke timeout")
		default:
			fmt.Printf("unexpected error: %v\n", err)
		}
		return err
	}

	fmt.Printf("got %v results\n", len(ack.Results))

	for _, accessResult := range ack.Results {

		fmt.Printf("Object ID: %s,", accessResult.ObjectIdentifier.String())

		for _, propResult := range accessResult.Results {
			if propResult.Error != nil {
				// Per-property error (device returned an error for this specific property).
				fmt.Printf("; %s: per-property error (raw: %x)\n",
					propResult.PropertyIdentifier, propResult.Error)
				continue
			}

			val, _, err := encoding.DecodeApplicationValue(propResult.PropertyValue, 0)
			if err != nil {
				fmt.Printf("; %s: per-property error (raw: %x)\n")
			}

			fmt.Printf("  %s: %v", propResult.PropertyIdentifier, val)
		}

		fmt.Printf("\n")
	}

	return nil
}
