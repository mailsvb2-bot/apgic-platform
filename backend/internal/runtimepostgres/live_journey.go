package runtimepostgres

import (
    "context"
    "database/sql"
    "fmt"
    "time"

    "github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
    "github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
)

// LoadLiveJourney preserves canonical booking/hold readback across API restarts,
// even if a specialist has subsequently unpublished an offer. It never hydrates
// conformance catalog economic fixtures or makes them payable.
func (c *Checker) LoadLiveJourney() (demand.JourneySnapshot, error) {
    snapshot,err:=c.LoadJourney(nil)
    if err!=nil{return snapshot,err}
    ctx,cancel:=context.WithTimeout(context.Background(),journeyWriteTimeout)
    defer cancel()
    tx,err:=c.db.BeginTx(ctx,&sql.TxOptions{ReadOnly:true,Isolation:sql.LevelRepeatableRead})
    if err!=nil{return snapshot,err}
    defer tx.Rollback()

    holds,err:=tx.QueryContext(ctx,`
      SELECT h.id::text,offer.specialist_id::text,s.starts_at,
             h.client_identity_id::text,h.state,h.expires_at
        FROM booking_holds h
        JOIN booking_slots s ON s.id=h.slot_id
        JOIN specialist_bookable_offers offer ON offer.id=s.specialist_offer_id
       WHERE s.tenant_scope='specialist/live'
       ORDER BY h.created_at,h.id`)
    if err!=nil{return snapshot,fmt.Errorf("read live holds: %w",err)}
    for holds.Next(){
        var hold demand.Hold
        var specialistID string
        var start time.Time
        if err:=holds.Scan(&hold.ID,&specialistID,&start,&hold.ClientIdentityID,&hold.State,&hold.ExpiresAt);err!=nil{
            holds.Close();return snapshot,err
        }
        hold.SlotID=publicSlotRef(specialistID,start)
        hold.BookingID,err=demandBookingIDForHold(hold.ID)
        if err!=nil{holds.Close();return snapshot,err}
        hold.BookingState=booking.StateHeld
        if hold.State=="EXPIRED"{hold.BookingState=booking.StateExpired}
        hold.ReasonCode="BOOK_HOLD_ACQUIRED"
        snapshot.Holds=append(snapshot.Holds,&hold)
    }
    if err:=holds.Err();err!=nil{holds.Close();return snapshot,err}
    holds.Close()

    bookings,err:=tx.QueryContext(ctx,`
      SELECT b.id::text,offer.specialist_id::text,b.hold_id::text,b.client_identity_id::text,
             b.state,b.hold_expires_at,b.starts_at,b.ends_at,b.updated_at
        FROM bookings b
        JOIN booking_slots s ON s.id=b.slot_id
        JOIN specialist_bookable_offers offer ON offer.id=s.specialist_offer_id
       WHERE s.tenant_scope='specialist/live'
       ORDER BY b.created_at,b.id`)
    if err!=nil{return snapshot,fmt.Errorf("read live bookings: %w",err)}
    for bookings.Next(){
        var booked booking.Booking
        var specialistID string
        var state string
        if err:=bookings.Scan(&booked.ID,&specialistID,&booked.HoldID,&booked.ClientIdentityID,
          &state,&booked.HoldExpiresAt,&booked.StartsAt,&booked.EndsAt,&booked.UpdatedAt);err!=nil{
            bookings.Close();return snapshot,err
        }
        booked.SlotID=publicSlotRef(specialistID,booked.StartsAt)
        booked.State=booking.State(state)
        snapshot.Bookings=append(snapshot.Bookings,&booked)
    }
    if err:=bookings.Err();err!=nil{bookings.Close();return snapshot,err}
    bookings.Close()
    if err:=tx.Commit();err!=nil{return snapshot,err}
    return snapshot,nil
}
