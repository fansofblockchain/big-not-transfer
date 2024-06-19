package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/r3labs/sse/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xssnick/tonutils-go/tlb"
)

// SSE endpoint of TONAPI
const SSE_ENDPOINT = "https://tonapi.io/v2/sse/accounts/transactions?accounts=ALL&operations="

// Operations that matters
// https://github.com/tonkeeper/tongo/blob/master/abi/messages.md
var TRANSFER_OPERATIONS = []string{
	"JettonTransfer",
}

var (
	logLevel  = flag.String("log-level", "info", "Log level")
	minTonCnt = flag.String("min", "100", "Minimum amount of NOT to be considered as transfer")
)

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *logLevel == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}

func main() {
	var url = fmt.Sprintf("%s%s", SSE_ENDPOINT, strings.Join(TRANSFER_OPERATIONS, ","))

	notTransfer := NewNotTransfer(tlb.MustFromTON(*minTonCnt))
	go notTransfer.Run()

	client := sse.NewClient(url)
	err := client.Subscribe("messages", func(msg *sse.Event) {
		m := Message{}
		err := json.Unmarshal(msg.Data, &m)
		if err != nil {
			log.Err(err).Msg("Failed to unmarshal message")
			return
		}

		notTransfer.messageChan <- &m
	})

	if err != nil {
		log.Err(err).Msg("Failed to subscribe to SSE")
	}
}
