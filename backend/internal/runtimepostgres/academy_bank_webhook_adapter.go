package runtimepostgres

import (
 "context"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

// AcademyBankCommitNotification is the existing Commerce capture writer's
// narrow bank-HTTP adapter. A signed notification AND fresh bank GetState
// must succeed before one atomic ledger/entitlement transaction can commit.
func (c *Checker) AcademyBankCommitNotification(ctx context.Context,orderID string,
 notification []byte,bank *payments.TBankAcquiring)error{
 _,err:=c.AcademyCommitBankCapture(ctx,orderID,notification,bank)
 return err
}
