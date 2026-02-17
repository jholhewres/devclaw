package copilot

import (
	"testing"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

func makeMsg(from, chatID string, isGroup bool) *channels.IncomingMessage {
	return &channels.IncomingMessage{
		From:    from,
		ChatID:  chatID,
		IsGroup: isGroup,
	}
}

func TestAccess_OwnerAllowed(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		Owners:        []string{"owner@s.whatsapp.net"},
		DefaultPolicy: PolicyDeny,
	}, nil)

	r := am.Check(makeMsg("owner@s.whatsapp.net", "owner@s.whatsapp.net", false))
	if !r.Allowed {
		t.Error("owner should be allowed")
	}
	if r.Level != AccessOwner {
		t.Errorf("expected AccessOwner, got %v", r.Level)
	}
}

func TestAccess_AdminAllowed(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		Admins:        []string{"admin@s.whatsapp.net"},
		DefaultPolicy: PolicyDeny,
	}, nil)

	r := am.Check(makeMsg("admin@s.whatsapp.net", "admin@s.whatsapp.net", false))
	if !r.Allowed {
		t.Error("admin should be allowed")
	}
	if r.Level != AccessAdmin {
		t.Errorf("expected AccessAdmin, got %v", r.Level)
	}
}

func TestAccess_UserAllowed(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		AllowedUsers:  []string{"user@s.whatsapp.net"},
		DefaultPolicy: PolicyDeny,
	}, nil)

	r := am.Check(makeMsg("user@s.whatsapp.net", "user@s.whatsapp.net", false))
	if !r.Allowed {
		t.Error("user should be allowed")
	}
	if r.Level != AccessUser {
		t.Errorf("expected AccessUser, got %v", r.Level)
	}
}

func TestAccess_BlockedRejected(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		BlockedUsers:  []string{"bad@s.whatsapp.net"},
		DefaultPolicy: PolicyAllow,
	}, nil)

	r := am.Check(makeMsg("bad@s.whatsapp.net", "bad@s.whatsapp.net", false))
	if r.Allowed {
		t.Error("blocked user should be rejected")
	}
	if r.Level != AccessBlocked {
		t.Errorf("expected AccessBlocked, got %v", r.Level)
	}
}

func TestAccess_UnknownDenyPolicy(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		DefaultPolicy: PolicyDeny,
	}, nil)

	r := am.Check(makeMsg("random@s.whatsapp.net", "random@s.whatsapp.net", false))
	if r.Allowed {
		t.Error("unknown contact should be denied with PolicyDeny")
	}
}

func TestAccess_UnknownAllowPolicy(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		DefaultPolicy: PolicyAllow,
	}, nil)

	r := am.Check(makeMsg("random@s.whatsapp.net", "random@s.whatsapp.net", false))
	if !r.Allowed {
		t.Error("unknown contact should be allowed with PolicyAllow")
	}
	if r.Level != AccessUser {
		t.Errorf("expected AccessUser for allow-policy unknown, got %v", r.Level)
	}
}

func TestAccess_UnknownAskPolicy(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		DefaultPolicy: PolicyAsk,
	}, nil)

	r := am.Check(makeMsg("new@s.whatsapp.net", "new@s.whatsapp.net", false))
	if r.Allowed {
		t.Error("first ask should not be allowed")
	}
	if !r.ShouldAsk {
		t.Error("should ask for first contact")
	}

	// After marking as asked, should NOT ask again.
	am.MarkAsked("new@s.whatsapp.net")
	r2 := am.Check(makeMsg("new@s.whatsapp.net", "new@s.whatsapp.net", false))
	if r2.Allowed {
		t.Error("should still be denied after asking")
	}
	if r2.ShouldAsk {
		t.Error("should NOT ask again")
	}
}

func TestAccess_GroupAllowed(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		AllowedGroups: []string{"group@g.us"},
		DefaultPolicy: PolicyDeny,
	}, nil)

	r := am.Check(makeMsg("someone@s.whatsapp.net", "group@g.us", true))
	if !r.Allowed {
		t.Error("allowed group should grant access")
	}
	if r.Level != AccessUser {
		t.Errorf("expected AccessUser from group, got %v", r.Level)
	}
}

func TestAccess_GroupBlocked(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		BlockedGroups: []string{"badgroup@g.us"},
		DefaultPolicy: PolicyAllow,
	}, nil)

	r := am.Check(makeMsg("someone@s.whatsapp.net", "badgroup@g.us", true))
	if r.Allowed {
		t.Error("blocked group should be rejected")
	}
}

func TestAccess_JIDNormalization(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		Owners:        []string{"5511999999999:5@s.whatsapp.net"},
		DefaultPolicy: PolicyDeny,
	}, nil)

	// Message from same user but without device suffix.
	r := am.Check(makeMsg("5511999999999@s.whatsapp.net", "5511999999999@s.whatsapp.net", false))
	if !r.Allowed {
		t.Error("JID normalization should strip device suffix")
	}
}

func TestAccess_GrantAndRevoke(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		DefaultPolicy: PolicyDeny,
	}, nil)

	jid := "grantee@s.whatsapp.net"

	// Grant user access.
	if err := am.Grant(jid, AccessUser, "admin"); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	r := am.Check(makeMsg(jid, jid, false))
	if !r.Allowed {
		t.Error("granted user should be allowed")
	}

	// Revoke access.
	am.Revoke(jid, "admin")

	r2 := am.Check(makeMsg(jid, jid, false))
	if r2.Allowed {
		t.Error("revoked user should be denied")
	}
}

func TestAccess_CannotGrantOwner(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{}, nil)
	err := am.Grant("user@s.whatsapp.net", AccessOwner, "admin")
	if err == nil {
		t.Error("should not be able to grant owner via Grant()")
	}
}

func TestAccess_BlockedUserOverridesGroupAllow(t *testing.T) {
	t.Parallel()
	am := NewAccessManager(AccessConfig{
		AllowedGroups: []string{"group@g.us"},
		BlockedUsers:  []string{"bad@s.whatsapp.net"},
		DefaultPolicy: PolicyDeny,
	}, nil)

	r := am.Check(makeMsg("bad@s.whatsapp.net", "group@g.us", true))
	if r.Allowed {
		t.Error("blocked user should be denied even in allowed group")
	}
}
