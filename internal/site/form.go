package site

type Form struct {
	m map[string]string
	v map[string]string
}

func newForm() Form {
	return Form{
		m: make(map[string]string, 0),
		v: make(map[string]string, 0),
	}
}

func (f Form) Add(field, value string) {
	f.v[field] = value
}

func (f Form) AddError(field, message string) {
	f.m[field] = message
}

func (f Form) Error() bool {
	return len(f.m) > 0
}

func (f Form) HasError(field string) bool {
	_, ok := f.m[field]
	return ok
}

func (f Form) Field(field string) string {
	return f.v[field]
}

func (f Form) FieldError(field string) string {
	return f.m[field]
}

func (f Form) AllErrors() []string {
	all := make([]string, 0, len(f.m))
	for _, m := range f.m {
		all = append(all, m)
	}
	return all
}
