package runtimepostgres

import (
	"context"
	"errors"
)

// IsAccountAccessRevoked is consulted by the existing canonical session
// validator, not by a competing authentication store. Database errors fail
// closed at the caller; an unavailable revocation ledger cannot grant access.
func (c *Checker) IsAccountAccessRevoked(ctx context.Context, identity string) (bool,error) {
	if c==nil||c.db==nil{return false,errors.New("postgres unavailable")}
	ctx,cancel:=context.WithTimeout(ctx,clientPersonalTimeout)
	defer cancel()
	var revoked bool
	err:=c.db.QueryRowContext(ctx,`SELECT EXISTS(
		SELECT 1 FROM client_deletion_revocations WHERE identity_id=$1::uuid
	)`,identity).Scan(&revoked)
	return revoked,err
}
