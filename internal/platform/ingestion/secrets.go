package ingestion

import "slices"

// SecretNames lists the secrets a source names, by walking the typed model:
// the credentials of each built-in kind, the auth of an http or api-poll
// source, and for a runtime-supplied kind the text settings (the platform
// cannot tell which of an extension's settings are secret names, and a name
// that is no secret reveals nothing). The runtime reads only these.
func SecretNames(src Source) []string {
	var names []string
	add := func(refs ...SecretRef) {
		for _, ref := range refs {
			names = append(names, ref.Name)
		}
	}
	switch s := src.(type) {
	case Sftp:
		add(s.Credential)
	case S3:
		if s.Credential != nil {
			add(s.Credential.SecretAccessKey)
		}
	case Sqs:
		if s.Credential != nil {
			add(s.Credential.SecretAccessKey)
		}
	case RabbitMQ:
		add(s.Password)
	case DatabasePoll:
		add(s.Password)
	case HTTP:
		switch a := s.Auth.(type) {
		case BasicAuth:
			add(a.Password)
		case BearerAuth:
			add(a.Token)
		}
	case APIPoll:
		switch a := s.Auth.(type) {
		case APIBasic:
			add(a.Password)
		case APIBearer:
			add(a.Token)
		case APIKey:
			add(a.Value)
		case ClientCredentials:
			add(a.ClientSecret)
		case Login:
			add(a.Password)
		}
	case Custom:
		for _, value := range s.Settings {
			switch x := value.(type) {
			case string:
				names = append(names, x)
			case []any:
				for _, item := range x {
					if text, ok := item.(string); ok {
						names = append(names, text)
					}
				}
			case []string:
				names = append(names, x...)
			}
		}
	}
	return slices.DeleteFunc(names, func(n string) bool { return n == "" })
}
