package main

import (
	"context"
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
	bacencoding "go.wdy.de/bacnet/encoding"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
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

	services, err := rt.Client().Discover(context.Background(), apdu.DiscoverRequest{
		Destination: netprim.NewAddressFromAddrPort(broadcastAddr),
		WhoIs:       apdu.NewWhoIsRequest(),
		Window:      time.Second * 5,
	})
	if err != nil {
		log.Printf("device not reachable")
		return err
	}

	if len(services) < 1 {
		return fmt.Errorf("no services found")
	}

	objId, err := types.NewObjectIdentifier(types.ObjectTypeAnalogValue, 270)
	if err != nil {
		panic(err)
	}

	// Use EncodeApplicationValue to encode a value
	enc, err := bacencoding.EncodeApplicationValue(bacencoding.AppReal(10.0))
	if err != nil {
		panic(err)
	}

	writeReq := apdu.WritePropertyRequest{
		ObjectIdentifier:   objId,
		PropertyIdentifier: types.PropertyIdentifierPresentValue,
		ArrayIndex:         nil,
		PropertyValue:      enc, // Example value to write
		Priority:           nil,
	}

	fmt.Printf("Writing Request to device: %v\n", writeReq)

	err = rt.Client().WriteProperty(context.Background(), services[0].Source, writeReq)
	if err != nil {
		return err
	}

	return nil
}
