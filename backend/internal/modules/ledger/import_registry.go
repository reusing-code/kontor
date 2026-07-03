package ledger

import "fmt"

var providers = map[SourceType]Provider{}

func RegisterProvider(p Provider) {
	providers[p.SourceType()] = p
}

func GetProvider(st SourceType) (Provider, error) {
	p, ok := providers[st]
	if !ok {
		return nil, fmt.Errorf("unknown source type: %s", st)
	}
	return p, nil
}

func RegisteredSourceTypes() []SourceType {
	types := make([]SourceType, 0, len(providers))
	for st := range providers {
		types = append(types, st)
	}
	return types
}
