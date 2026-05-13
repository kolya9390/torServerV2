package torr

import (
	"context"
	"time"

	"github.com/anacrolix/torrent"

	"server/torr/utils"
)

func (bt *BTServer) Connect() error {
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bt.configure(ctx)
	bt.client, err = torrent.NewClient(bt.config)
	bt.registry.Reset()

	return err
}

func (bt *BTServer) Disconnect() {
	if bt.client != nil {
		bt.client.Close()
		bt.client = nil

		utils.FreeOSMemGC()
	}
}
