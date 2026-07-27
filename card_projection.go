package soothe

import "strings"

// ParsedCardFrame is one soothe.card.* custom payload after peel.
type ParsedCardFrame struct {
	WireType string
	Card     map[string]interface{}
	Patch    map[string]interface{}
}

var cardMutationTypes = map[string]struct{}{
	EventCardCreated:     {},
	EventCardUpdated:     {},
	EventCardFinalized:   {},
	EventCardReplayBegin: {},
	EventCardReplayEnd:   {},
}

// ParseCardCustomPayload parses a custom-mode card frame, or nil if not a card frame.
func ParseCardCustomPayload(data interface{}) *ParsedCardFrame {
	m, ok := data.(map[string]interface{})
	if !ok || m == nil {
		return nil
	}
	wireType := strings.TrimSpace(asString(m["type"]))
	if _, ok := cardMutationTypes[wireType]; !ok {
		return nil
	}
	if wireType == EventCardReplayBegin || wireType == EventCardReplayEnd {
		return &ParsedCardFrame{WireType: wireType, Patch: map[string]interface{}{}}
	}
	payload := map[string]interface{}{}
	if raw, ok := m["data"].(map[string]interface{}); ok && raw != nil {
		for k, v := range raw {
			payload[k] = v
		}
	}
	cardID := strings.TrimSpace(asString(m["card_id"]))
	if cardID == "" {
		cardID = strings.TrimSpace(asString(payload["id"]))
	}
	if wireType == EventCardCreated {
		if payload["type"] == nil || payload["content"] == nil {
			return nil
		}
		if strings.TrimSpace(asString(payload["id"])) == "" && cardID != "" {
			payload["id"] = cardID
		}
		return &ParsedCardFrame{WireType: wireType, Card: payload, Patch: map[string]interface{}{}}
	}
	if strings.TrimSpace(asString(payload["id"])) == "" && cardID != "" {
		payload["id"] = cardID
	}
	return &ParsedCardFrame{WireType: wireType, Patch: payload}
}

// CardProjection is an in-memory card_id → wire dict map driven by soothe.card.* frames.
type CardProjection struct {
	cards     map[string]map[string]interface{}
	order     []string
	replaying bool
}

// NewCardProjection constructs an empty projection.
func NewCardProjection() *CardProjection {
	return &CardProjection{cards: map[string]map[string]interface{}{}}
}

// Replaying reports whether the projection is between replay.begin and replay.end.
func (p *CardProjection) Replaying() bool {
	return p.replaying
}

// Snapshot returns cards in insertion order.
func (p *CardProjection) Snapshot() []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(p.order))
	for _, id := range p.order {
		if c, ok := p.cards[id]; ok {
			out = append(out, c)
		}
	}
	return out
}

// Get returns one card by id.
func (p *CardProjection) Get(cardID string) map[string]interface{} {
	return p.cards[cardID]
}

// Apply one custom-mode card payload. Returns true when handled.
func (p *CardProjection) Apply(data interface{}) bool {
	parsed := ParseCardCustomPayload(data)
	if parsed == nil {
		return false
	}
	switch parsed.WireType {
	case EventCardReplayBegin:
		p.replaying = true
		p.cards = map[string]map[string]interface{}{}
		p.order = nil
		return true
	case EventCardReplayEnd:
		p.replaying = false
		return true
	case EventCardCreated:
		if parsed.Card == nil {
			return true
		}
		id := strings.TrimSpace(asString(parsed.Card["id"]))
		if id == "" {
			return true
		}
		if _, exists := p.cards[id]; !exists {
			p.order = append(p.order, id)
		}
		cp := map[string]interface{}{}
		for k, v := range parsed.Card {
			cp[k] = v
		}
		p.cards[id] = cp
		return true
	case EventCardUpdated, EventCardFinalized:
		id := strings.TrimSpace(asString(parsed.Patch["id"]))
		if id == "" {
			return true
		}
		existing, ok := p.cards[id]
		if !ok {
			return true
		}
		next := map[string]interface{}{}
		for k, v := range existing {
			next[k] = v
		}
		for k, v := range parsed.Patch {
			if k == "id" || k == "type" {
				continue
			}
			next[k] = v
		}
		p.cards[id] = next
		return true
	}
	return true
}
