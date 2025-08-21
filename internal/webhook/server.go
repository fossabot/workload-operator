package webhook

import (
	"context"
	"fmt"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type clusterAwareWebhookServer struct {
	webhook.Server
}

var _ webhook.Server = &clusterAwareWebhookServer{}

func (s *clusterAwareWebhookServer) Register(path string, hook http.Handler) {
	if h, ok := hook.(*admission.Webhook); ok {
		orig := h.Handler
		h.Handler = admission.HandlerFunc(func(ctx context.Context, req admission.Request) admission.Response {
			if c := clusterFromExtra(req.UserInfo.Extra); c != "" {
				ctx = WithClusterName(ctx, c)
			}
			return orig.Handle(ctx, req)
		})
	}

	path = fmt.Sprintf("/clusters/{cluster_name}%s", path)

	s.Server.Register(path, hook)
}

func NewClusterAwareWebhookServer(server webhook.Server) webhook.Server {
	return &clusterAwareWebhookServer{
		Server: server,
	}
}
