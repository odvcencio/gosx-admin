package mdppstudio_test

import (
	"testing"

	"m31labs.dev/gosx-admin/mdppstudio"
	"m31labs.dev/gosx-admin/mdppstudio/draftstoretest"
)

func TestMemoryDraftStoreConformance(t *testing.T) {
	draftstoretest.Run(t, func(t *testing.T) draftstoretest.Harness {
		store := mdppstudio.NewMemoryDraftStore()
		return draftstoretest.Harness{Store: store, Reopen: func() mdppstudio.DraftStore { return store }}
	})
}
