package runtimepostgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/searchdistribution"
)

// PublicTopicOffers is a rebuildable, read-only projection. The immutable
// business truth remains in specialist, qualification, Product and booking.
func (c *Checker) PublicTopicOffers(topic string) ([]searchdistribution.Offer,error) {
	if c==nil||c.db==nil{return nil,errors.New("search distribution unavailable")}
	ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
	defer cancel()
	rows,err:=c.db.QueryContext(ctx,`SELECT offer.id::text,sp.id::text,
	sp.display_name,sp.profession_code,offer.price_minor,offer.currency,
	EXISTS(SELECT 1 FROM booking_slots slot
	  WHERE slot.specialist_offer_id=offer.id
	    AND slot.tenant_scope='specialist/live'
	    AND slot.exclusive AND slot.starts_at>clock_timestamp()+interval '15 minutes'
	    AND NOT EXISTS(SELECT 1 FROM booking_holds hold
	      WHERE hold.slot_id=slot.id AND hold.state='ACTIVE' AND hold.expires_at>clock_timestamp())
	    AND NOT EXISTS(SELECT 1 FROM bookings b
	      WHERE b.slot_id=slot.id AND b.state IN ('HELD','PENDING_PAYMENT','CONFIRMED'))
	) AS has_slot
	FROM specialist_bookable_offers offer
	JOIN specialist_profiles sp ON sp.id=offer.specialist_id
	WHERE offer.topic_id=$1 AND apgic_live_offer_valid(offer.id)
	ORDER BY sp.id,offer.id LIMIT 50`,topic)
	if err!=nil{return nil,fmt.Errorf("read public topic projection: %w",err)}
	defer rows.Close()
	out:=make([]searchdistribution.Offer,0)
	for rows.Next(){
		var x searchdistribution.Offer
		if err:=rows.Scan(&x.OfferID,&x.SpecialistID,&x.DisplayName,&x.Profession,&x.PriceMinor,&x.Currency,&x.HasAvailableSlot);err!=nil{return nil,err}
		out=append(out,x)
	}
	return out,rows.Err()
}
