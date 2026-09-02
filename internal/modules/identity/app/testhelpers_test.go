package app

import (
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"billing-platform/internal/platform/permissions"
)

func principalOf(orgID, userID uuid.UUID) permissions.Principal {
	return permissions.Principal{OrganisationID: orgID, UserID: userID}
}

func principalFor(r LoginResult) permissions.Principal {
	return permissions.Principal{OrganisationID: r.OrganisationID, UserID: r.UserID}
}

func totpCodeFor(secret string) (string, error) {
	return totp.GenerateCode(secret, time.Now())
}
