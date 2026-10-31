package academy

// AuthorProduct is a read-only projection of a published canonical Commerce
// product that the signed Academy author may use to create a course.
type AuthorProduct struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
