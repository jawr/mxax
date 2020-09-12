package controlpanel

type FormErrors struct {
	m map[string]string
}

func newFormErrors() FormErrors {
	return FormErrors{
		m: make(map[string]string, 0),
	}
}

func (f FormErrors) Add(field, message string) {
	f.m[field] = message
}

func (f FormErrors) Error() bool {
	return len(f.m) > 0
}

func (f FormErrors) HasError(field string) bool {
	_, ok := f.m[field]
	return ok
}

func (f FormErrors) Field(field string) string {
	return f.m[field]
}

func (f FormErrors) All() []string {
	all := make([]string, 0, len(f.m))
	for _, m := range f.m {
		all = append(all, m)
	}
	return all
}

func (f FormErrors) Clear() {
	f.m = make(map[string]string, 0)
}
