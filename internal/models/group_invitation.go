package models

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"fmt"
	"satellity/internal/durable"
	"satellity/internal/session"
	"strings"
	"time"

	"github.com/gofrs/uuid"
)

const groupInvitationsDDL = `
CREATE TABLE IF NOT EXISTS group_invitations (
	invitation_id          VARCHAR(36) PRIMARY KEY,
	group_id               VARCHAR(36) NOT NULL REFERENCES groups ON DELETE CASCADE,
	email                  VARCHAR(512) NOT NULL,
	code                   VARCHAR(128) NOT NULL,
	sent_at                TIMESTAMP WITH TIME ZONE,
	created_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS group_invitations_group_emailx ON group_invitations (group_id, email);
`

const (
	MaxGroupInvitations = 7
)

// GroupInvitation is a way to invate user to group for free
type GroupInvitation struct {
	InvitationID string
	GroupID      string
	Email        string
	Code         string
	SentAt       time.Time
	CreatedAt    time.Time
}

var groupInvitationColumns = []string{"invitation_id", "group_id", "email", "code", "sent_at", "created_at"}

func (i *GroupInvitation) values() []interface{} {
	return []interface{}{i.InvitationID, i.GroupID, i.Email, i.Code, i.SentAt, i.CreatedAt}
}

func groupInvitationFromRows(row durable.Row) (*GroupInvitation, error) {
	var i GroupInvitation
	err := row.Scan(&i.InvitationID, &i.GroupID, &i.Email, &i.Code, &i.SentAt, &i.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &i, err
}

// CreateGroupInvitation create a group invitation by email
func (user *User) CreateGroupInvitation(mctx *Context, groupID, email string) (*GroupInvitation, error) {
	ctx := mctx.context

	var invitation *GroupInvitation
	err := mctx.database.RunInTransaction(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT count(*) FROM group_invitations WHERE group_id=$1", groupID)
		var count int64
		err := row.Scan(&count)
		if err != nil {
			return err
		}
		if count > 7 {
			return session.TooManyGroupInvitationsError(ctx)
		}
		group, err := findGroup(ctx, tx, groupID)
		if err != nil {
			return err
		} else if group == nil {
			return nil
		} else if user.UserID != group.UserID {
			return session.ForbiddenError(ctx)
		}

		invitation = &GroupInvitation{
			InvitationID: uuid.Must(uuid.NewV4()).String(),
			GroupID:      group.GroupID,
			Email:        email,
			CreatedAt:    time.Now(),
		}
		invitation.Code, err = generateVerificationCode(ctx)
		if err != nil {
			return err
		}
		columns, params := durable.PrepareColumnsWithValues(groupInvitationColumns)
		_, err = tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO group_invitations(%s) VALUES (%s)", columns, params), invitation.values()...)
		return err
	})
	if err != nil {
		if _, ok := err.(session.Error); ok {
			return nil, err
		}
		return nil, session.TransactionError(ctx, err)
	}
	return invitation, nil
}

// JoinGroupByInvitation join the group by invitation code
func (user *User) JoinGroupByInvitation(mctx *Context, groupID, code string) (*Group, error) {
	ctx := mctx.context
	var group *Group
	err := mctx.database.RunInTransaction(ctx, func(tx *sql.Tx) error {
		var err error
		group, err = findGroup(ctx, tx, groupID)
		if err != nil || group == nil {
			return err
		}
		invitation, err := findGroupInvitationByGroupIDAndEmail(ctx, tx, group.GroupID, user.Email.String)
		if err != nil || invitation == nil {
			return err
		}
		if invitation.Code != strings.TrimSpace(code) {
			return session.InvalidGroupInvitationCodeError(ctx)
		}
		owner, err := findUserByID(ctx, tx, group.UserID)
		if err != nil {
			return err
		}
		group.User = owner

		var count int64
		err = tx.QueryRowContext(ctx, "SELECT count(*) FROM participants WHERE group_id=$1", groupID).Scan(&count)
		if err != nil {
			return err
		}
		group.UsersCount = count + 1
		_, err = tx.ExecContext(ctx, "UPDATE groups SET users_count=$1 WHERE group_id=$2", group.UsersCount, group.GroupID)
		if err != nil {
			return err
		}

		group.Role = ParticipantRoleVIP
		_, err = createParticipant(ctx, tx, group, user.UserID, ParticipantSourceInvitation)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM group_invitations WHERE invitation_id=$1", invitation.InvitationID)
		return err
	})
	if err != nil {
		if _, ok := err.(session.Error); ok {
			return nil, err
		}
		return nil, session.TransactionError(ctx, err)
	}
	return group, nil
}

func findGroupInvitationByGroupIDAndEmail(ctx context.Context, tx *sql.Tx, groupID, email string) (*GroupInvitation, error) {
	query := fmt.Sprintf("SELECT %s FROM group_invitations WHERE group_id=$1 AND email=$2 LIMIT 1", strings.Join(groupInvitationColumns, ","))
	row := tx.QueryRowContext(ctx, query, groupID, email)
	return groupInvitationFromRows(row)
}

func generateVerificationCode(ctx context.Context) (string, error) {
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", session.ServerError(ctx, err)
	}
	c := binary.LittleEndian.Uint64(b[:]) % 10000
	if c < 1000 {
		c = 1000 + c
	}
	return fmt.Sprint(c), nil
}
