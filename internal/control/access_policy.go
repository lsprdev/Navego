package control

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/lsprdev/Navego/pb_migrations"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

type accessPolicy struct{ emails map[string]bool }

func parseAccessPolicy(raw string) (*accessPolicy, error) {
	p := &accessPolicy{emails: make(map[string]bool)}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		email := strings.ToLower(strings.TrimSpace(part))
		if email == "" {
			continue
		}
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email || strings.ContainsAny(email, "*<>") {
			return nil, fmt.Errorf("NAVEGO_ALLOWED_EMAILS deve conter somente endereços de e-mail completos")
		}
		p.emails[email] = true
	}
	return p, nil
}

func (p *accessPolicy) allowsEmail(email string) bool {
	return p != nil && p.emails[strings.ToLower(strings.TrimSpace(email))]
}

func (p *accessPolicy) allowsUser(user *core.Record) bool {
	return user != nil && user.Verified() && p.allowsEmail(user.Email())
}

func bindAccessPolicy(app core.App, p *accessPolicy) {
	// Deleting a user cascades its database rows, but not Docker containers.
	// Keep capacity reserved until the agent has actually removed each browser.
	app.OnRecordDeleteRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		originalApp := e.App
		defer func() { e.App = originalApp }()
		return originalApp.RunInTransaction(func(tx core.App) error {
			e.App = tx
			count, err := tx.CountRecords(pb_migrations.BrowsersCollection, dbx.HashExp{"owner": e.Record.Id})
			if err != nil {
				return err
			}
			if count > 0 {
				return e.BadRequestError("Exclua os navegadores pelo Navego e aguarde a conclusão antes de excluir a conta.", nil)
			}
			return e.Next()
		})
	})
	app.OnRecordCreateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if !p.allowsEmail(e.Record.Email()) {
			return e.ForbiddenError("Cadastro restrito a e-mails autorizados pelo administrador.", nil)
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if !p.allowsEmail(e.Record.Email()) {
			return e.ForbiddenError("Este e-mail não está autorizado.", nil)
		}
		return e.Next()
	})
	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if !p.allowsUser(e.Record) {
			return e.ForbiddenError("Acesso restrito a e-mails autorizados e verificados.", nil)
		}
		return e.Next()
	})
}

// Runs after PocketBase loads the auth record, including for native collection
// APIs. Previously issued dashboard tokens cannot bypass a changed allowlist.
func accessMiddleware(p *accessPolicy) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id: "navegoEmailAccess", Priority: apis.DefaultLoadAuthTokenMiddlewarePriority + 1,
		Func: func(e *core.RequestEvent) error {
			if e.Auth != nil && e.Auth.Collection().Name == "users" && !p.allowsUser(e.Auth) {
				return e.ForbiddenError("Acesso restrito a e-mails autorizados e verificados.", nil)
			}
			return e.Next()
		},
	}
}
