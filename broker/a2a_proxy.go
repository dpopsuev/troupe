package broker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"

	"github.com/dpopsuev/troupe/internal/transport"
)

// A2AProxyFactory returns a ProxyFactory that builds transport handlers
// backed by real A2A outbound client calls. When an admitted external
// agent receives a message, the handler sends it to the agent's
// callbackURL via a2a-go message/send.
func A2AProxyFactory() ProxyFactory {
	return func(callbackURL string) transport.MsgHandler {
		return func(ctx context.Context, msg transport.Message) (transport.Message, error) {
			card := a2a.AgentCard{
				URL:                callbackURL,
				PreferredTransport: a2a.TransportProtocolJSONRPC,
			}

			client, err := a2aclient.NewFromCard(ctx, &card, a2aclient.WithJSONRPCTransport(http.DefaultClient))
			if err != nil {
				slog.WarnContext(ctx, "a2a proxy: client creation failed",
					slog.String("url", callbackURL),
					slog.String("error", err.Error()),
				)
				return transport.Message{}, fmt.Errorf("a2a proxy client: %w", err)
			}

			a2aMsg := a2a.NewMessage(a2a.MessageRoleUser, &a2a.TextPart{Text: msg.Content})
			result, err := client.SendMessage(ctx, &a2a.MessageSendParams{
				Message: a2aMsg,
			})
			if err != nil {
				slog.WarnContext(ctx, "a2a proxy: send failed",
					slog.String("url", callbackURL),
					slog.String("error", err.Error()),
				)
				return transport.Message{}, fmt.Errorf("a2a proxy send: %w", err)
			}

			task, ok := result.(*a2a.Task)
			if !ok {
				return transport.Message{}, fmt.Errorf("a2a proxy: unexpected result type %T", result)
			}

			if task.Status.Message == nil {
				return transport.Message{Content: ""}, nil
			}

			resp := transport.FromA2AMessage(*task.Status.Message, msg.From)

			slog.InfoContext(ctx, "a2a proxy: response received",
				slog.String("url", callbackURL),
				slog.String("content_length", fmt.Sprintf("%d", len(resp.Content))),
			)

			return resp, nil
		}
	}
}
