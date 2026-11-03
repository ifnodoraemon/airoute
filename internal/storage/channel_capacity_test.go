package storage

import (
	"github.com/ifnodoraemon/airoute/internal/model"
	"testing"
)

func TestChannelCapacityRoundTripAndRetiredProtocolMigration(t *testing.T) {
	repo := newMigrationsRepo(t)
	ch := &ChannelRecord{Name: "capacity", Type: model.ProviderOpenAI, BaseURL: "https://unit.invalid", Models: []string{"m"}, Protocols: []string{"openai_chat", "openai_text"}, MaxConcurrency: 3, QueueLimit: 7, QueueWaitSeconds: 12, Status: "active"}
	if e := repo.CreateChannel(ch); e != nil {
		t.Fatal(e)
	}
	other := &ChannelRecord{Name: "retired", Type: model.ProviderOpenAI, BaseURL: "https://retired.invalid", Protocols: []string{"openai_text"}, Status: "active"}
	if e := repo.CreateChannel(other); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e := retireTextCompletions(repo.db); e != nil {
			t.Fatal(e)
		}
		if e := addChannelCapacity(repo.db); e != nil {
			t.Fatal(e)
		}
	}
	channels, e := repo.ListChannels()
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range channels {
		if row.ID == ch.ID && (row.MaxConcurrency != 3 || row.QueueLimit != 7 || row.QueueWaitSeconds != 12 || len(row.Protocols) != 1 || row.Status != "active") {
			t.Fatalf("mixed channel or capacity settings lost: %+v", row)
		}
		if row.ID == other.ID && row.Status != "disabled" {
			t.Fatal("retired-only channel became unrestricted")
		}
	}
	_, e = repo.PatchChannel(ch.ID, func(row *ChannelRecord) error {
		row.MaxConcurrency = 0
		row.QueueLimit = 0
		row.QueueWaitSeconds = 0
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	channels, e = repo.ListChannels()
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range channels {
		if row.ID == ch.ID && (row.MaxConcurrency != 0 || row.QueueLimit != 0 || row.QueueWaitSeconds != 0) {
			t.Fatal("explicit zero did not disable capacity limits")
		}
	}
}
