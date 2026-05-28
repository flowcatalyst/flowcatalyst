package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

const tagMessages = "messages"

func registerMessages(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "publishMessage", Method: http.MethodPost, Path: "/messages",
		Summary:       "Publish a message to a pool's queue",
		Description:   "Looks up the queue config bound to PoolCode and publishes via the matching backend (SQS / Postgres / ...). Reuses the same broker the consumer reads from.",
		Tags:          []string{tagMessages},
		DefaultStatus: http.StatusCreated,
	}, s.publishMessage)
}

type publishMessageInput struct {
	Body PublishMessageRequest
}

type publishMessageOutput struct {
	Body PublishMessageResponse
}

// publishMessage is the POST /messages handler. The caller supplies a
// PoolCode + the message body; the router looks up the pool's queue
// config (via Manager.QueueConfig) and publishes through a cached
// Publisher for that backend.
func (s *State) publishMessage(ctx context.Context, in *publishMessageInput) (*publishMessageOutput, error) {
	if s.Publisher == nil {
		return nil, notConfigured("publisher")
	}
	req := in.Body
	if err := req.validate(); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	msg := req.toMessage()
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}

	pub, err := s.Publisher.Publisher(ctx, msg.PoolCode)
	if err != nil {
		return nil, huma.Error502BadGateway("publisher: " + err.Error())
	}
	brokerID, err := pub.Publish(ctx, msg)
	if err != nil {
		slog.Warn("publish failed", "pool", msg.PoolCode, "msg_id", msg.ID, "err", err)
		return nil, huma.Error502BadGateway("publish: " + err.Error())
	}
	slog.Info("message published", "pool", msg.PoolCode, "msg_id", msg.ID, "broker_id", brokerID)
	return &publishMessageOutput{Body: PublishMessageResponse{
		MessageID:       msg.ID,
		BrokerMessageID: brokerID,
		PoolCode:        msg.PoolCode,
		QueueIdentifier: pub.Identifier(),
	}}, nil
}

// toMessage converts the wire request to a common.Message. Field
// defaults mirror what the router would receive from a normal broker
// payload — DispatchMode defaults to IMMEDIATE, MediationType defaults
// to HTTP.
func (r PublishMessageRequest) toMessage() common.Message {
	mediationType := common.MediationType(r.MediationType)
	if mediationType == "" {
		mediationType = common.MediationTypeHTTP
	}
	dispatchMode := common.DispatchMode(r.DispatchMode)
	if dispatchMode == "" {
		dispatchMode = common.DispatchImmediate
	}
	var msgGroup *string
	if r.MessageGroupID != "" {
		v := r.MessageGroupID
		msgGroup = &v
	}
	var authToken *string
	if r.AuthToken != "" {
		v := r.AuthToken
		authToken = &v
	}
	var signingSecret *string
	if r.SigningSecret != "" {
		v := r.SigningSecret
		signingSecret = &v
	}
	return common.Message{
		ID:              r.ID,
		PoolCode:        r.PoolCode,
		AuthToken:       authToken,
		SigningSecret:   signingSecret,
		MediationType:   mediationType,
		MediationTarget: r.MediationTarget,
		MessageGroupID:  msgGroup,
		HighPriority:    r.HighPriority,
		DispatchMode:    dispatchMode,
	}
}

func (r PublishMessageRequest) validate() error {
	if r.PoolCode == "" {
		return huma.Error400BadRequest("poolCode is required")
	}
	if r.MediationTarget == "" {
		return huma.Error400BadRequest("mediationTarget is required")
	}
	return nil
}
