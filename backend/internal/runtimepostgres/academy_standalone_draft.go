package runtimepostgres

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "strings"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyCreateStandaloneDraft writes a private, unsellable Commerce DRAFT and
// an Academy DRAFT in one PostgreSQL transaction. It never publishes a product,
// grants learning access, issues an entitlement, or creates a sale.
func (c *Checker) AcademyCreateStandaloneDraft(actor, title, description string) (academy.Course, error) {
 title = strings.TrimSpace(title)
 if !academyUUID(actor) || len(title)<3 || len(title)>200 {return academy.Course{},academy.ErrInvalid}
 ctx,cancel,tx,err:=academyBegin(c)
 defer cancel()
 if err!=nil{return academy.Course{},err}
 defer tx.Rollback()
 productID,err:=academyID();if err!=nil{return academy.Course{},err}
 courseID,err:=academyID();if err!=nil{return academy.Course{},err}
 versionID,err:=academyID();if err!=nil{return academy.Course{},err}
 ownerRef:="identity/"+actor
 _,err=tx.ExecContext(ctx,`INSERT INTO products(
 id,owner_type,owner_id,commercial_owner_ref,revenue_beneficiary_ref,author_refs,name,status)
 VALUES($1::uuid,'IDENTITY',$2::uuid,$3,$3,ARRAY[$3]::text[],$4,'DRAFT')`,
 productID,actor,ownerRef,title)
 if err!=nil{return academy.Course{},fmt.Errorf("create private course product draft: %w",err)}
 _,err=tx.ExecContext(ctx,`INSERT INTO academy_courses(id,product_id,author_id,title,description)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5)`,courseID,productID,actor,title,description)
 if err!=nil{return academy.Course{},fmt.Errorf("create academy draft: %w",err)}
 _,err=tx.ExecContext(ctx,`INSERT INTO academy_course_versions(id,course_id,version_no)
 VALUES($1::uuid,$2::uuid,1)`,versionID,courseID)
 if err!=nil{return academy.Course{},err}
 if err=academyEvent(ctx,tx,actor,"academy/course/"+courseID,"academy.course.created",
 map[string]string{"author_id":actor,"version_id":versionID});err!=nil{return academy.Course{},err}
 if err=tx.Commit();err!=nil{return academy.Course{},err}
 return academy.Course{ID:courseID,ProductID:productID,Title:title,Description:description,AuthorID:actor,
 State:"DRAFT",VersionID:versionID,VersionNo:1},nil
}
var _ = context.Background
var _ = sql.ErrNoRows
var _ = errors.Is
