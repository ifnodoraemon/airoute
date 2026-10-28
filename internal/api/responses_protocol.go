package api

import "github.com/ifnodoraemon/airoute/internal/model"

// responsesProtocol is the routing protocol tag for requests that arrived on the
// OpenAI Responses ingress. It selects channels that speak the response protocol
// during candidate resolution instead of falling back to plain chat.
const responsesProtocol = "responses"

// responsesChatRequest converts a Responses request into the canonical chat
// request and marks it as a response-protocol call.
//
// B-06: without the tag the dispatcher routed the request as an ordinary chat
// turn, so response-native channels were never preferred even though the model
// layer knows the pairing. The tag is internal metadata (json:"-") and is never
// serialized to the client.
func responsesChatRequest(req model.ResponseRequest) (*model.ChatCompletionRequest, error) {
	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		return nil, err
	}
	chatReq.Protocol = responsesProtocol
	return chatReq, nil
}
