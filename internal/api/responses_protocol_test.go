package api

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// B-06: a request that arrived on the Responses ingress must carry the response
// routing protocol into candidate selection; previously it was routed as chat.
func TestResponsesChatRequestTagsProtocol(t *testing.T) {
	req := model.ResponseRequest{
		Model: "gpt-4o",
		Input: "hello",
	}
	chatReq, err := responsesChatRequest(req)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if chatReq.Protocol != responsesProtocol {
		t.Fatalf("protocol = %q, want %q", chatReq.Protocol, responsesProtocol)
	}
	if chatReq.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", chatReq.Model)
	}
}

// The protocol tag must be the one the model layer recognises as a response
// protocol, otherwise channel selection ignores it.
func TestResponsesProtocolIsRecognisedByChannelMatching(t *testing.T) {
	ch := &model.ChannelConfig{Protocols: []string{"response"}}
	if !ch.SupportsProtocol(responsesProtocol) {
		t.Fatalf("a response-protocol channel must accept %q", responsesProtocol)
	}
}
