package runtimepostgres

import (
 "context"
 "errors"
 "os"
 "testing"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
 "github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

func TestPublicSpecialistFullPublicationReviewAndRemoval(t *testing.T) {
 dbURL:=os.Getenv("APGIC_SPECIALIST_TEST_DATABASE_URL")
 if dbURL==""{t.Skip("no specialist PostgreSQL integration database")}
 store,err:=Open(context.Background(),dbURL);if err!=nil{t.Fatal(err)};defer store.Close()
 ctx:=context.Background()
 identityID,err:=persistentid.New();if err!=nil{t.Fatal(err)}
 p,err:=store.UpsertProfile(identityID,"Мария Каталог","PSYCHOLOGIST");if err!=nil{t.Fatal(err)}
 if _,err=store.DeclareCapability(identityID,"anxiety");err!=nil{t.Fatal(err)}
 p,err=store.SubmitEvidence(identityID,"anxiety","DIPLOMA","document:public-directory-test");if err!=nil{t.Fatal(err)}
 if err=store.ReviewEvidence(p.Evidence[0].ID,true,"reviewer:public-test");err!=nil{t.Fatal(err)}
 if err=store.ReviewProfile(identityID,true,"reviewer:public-test");err!=nil{t.Fatal(err)}
 if _,err=store.Publish(identityID,"anxiety");err!=nil{t.Fatal(err)}
 listing,err:=store.PublicProfiles(ctx,"anxiety",10,0);if err!=nil{t.Fatal(err)}
 found:=false
 for _,item:=range listing{if item.ID==p.ID{found=true}}
 if !found{t.Fatal("approved published specialist missing from public directory")}
 details:=specialist.PublicDetails{Headline:"Сон и тревога",About:"Работаю онлайн",ExperienceYears:4,Languages:[]string{"ru"},Limitations:[]string{"Не работаю с детьми"}}
 if _,err=store.SavePublicDetails(ctx,identityID,details);err!=nil{t.Fatal(err)}
 if _,err=store.PublicProfile(ctx,p.ID);!errors.Is(err,specialist.ErrPublicProfileNotFound){t.Fatalf("edited unreviewed claims remained public: %v",err)}
 if err=store.ReviewProfile(identityID,true,"reviewer:public-test");err!=nil{t.Fatal(err)}
 if _,err=store.PublicProfile(ctx,p.ID);!errors.Is(err,specialist.ErrPublicProfileNotFound){t.Fatal("review alone resurrected old publication")}
 if res,err:=store.Publish(identityID,"anxiety");err!=nil||!res.Allowed{t.Fatalf("republication: %#v / %v",res,err)}
 got,err:=store.PublicProfile(ctx,p.ID);if err!=nil{t.Fatal(err)}
 if got.About!=details.About||len(got.PublishedTopics)!=1||got.ID!=p.ID{t.Fatalf("public profile mismatch: %#v",got)}
 if _,err=store.Unpublish(identityID,"anxiety");err!=nil{t.Fatal(err)}
 if _,err=store.PublicProfile(ctx,p.ID);!errors.Is(err,specialist.ErrPublicProfileNotFound){t.Fatal("unpublished profile remained visible")}
}
