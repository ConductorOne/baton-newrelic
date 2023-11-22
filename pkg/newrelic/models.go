package newrelic

// TODO: add more comments

type BaseResource struct {
	ID string `json:"id"`
}

type User struct {
	ID    string `json:"userId"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Org struct {
	BaseResource
	Name string `json:"name"`
}

type Domain struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	NextCursor string  `json:"nextCursor"`
	Total      int     `json:"totalCount"`
	Groups     []Group `json:"groups"`
}

type Group struct {
	BaseResource
	Name  string `json:"displayName"`
	Roles struct {
		NextCursor string `json:"nextCursor"`
		TotalCount int    `json:"totalCount"`
		Roles      []Role `json:"roles"`
	} `json:"roles"`
}

type Role struct {
	BaseResource
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
}
