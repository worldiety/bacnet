package npdu

import (
	"go.wdy.de/bacnet/common/log"
)

func decodeAndNormalizeNetworkLayerMessagePayload(header NetworkLayerMessageHeader, payload []byte) ([]byte, error) {

	log.Logger.Debug(
		"npdu normalize network-layer payload inbound",
		"message_type", uint8(header.MessageType),
		"has_vendor_id", header.VendorID != nil,
		"payload_bytes", len(payload),
	)

	message, err := DecodeNetworkLayerMessageModel(header, payload)
	if err != nil {
		return nil, err
	}

	normalizedPayload := message.PayloadBytes()
	log.Logger.Debug(
		"npdu normalize network-layer payload success",
		"message_type", uint8(message.Header().MessageType),
		"payload_bytes", len(normalizedPayload),
	)

	return normalizedPayload, nil
}

func validateNetworkLayerMessagePayload(header NetworkLayerMessageHeader, payload []byte) error {
	_, err := decodeAndNormalizeNetworkLayerMessagePayload(header, payload)
	return err
}
