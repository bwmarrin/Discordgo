package discordgo

import (
	"encoding/json"
)

// ComponentType is type of component.
type ComponentType uint

// MessageComponent types.
const (
	ActionsRowComponent ComponentType = 1
	ButtonComponent     ComponentType = 2
	SelectMenuComponent ComponentType = 3
)

// MessageComponent is a base interface for all message components.
type MessageComponent interface {
	json.Marshaler
	Type() ComponentType
}

// ActionsRow is a container for components within one row.
type ActionsRow struct {
	Components []MessageComponent `json:"components"`
}

// MarshalJSON is a method for marshaling ActionsRow to a JSON object.
func (r ActionsRow) MarshalJSON() ([]byte, error) {
	type actionRow ActionsRow

	return json.Marshal(struct {
		actionRow
		Type ComponentType `json:"type"`
	}{
		actionRow: actionRow(r),
		Type:      r.Type(),
	})
}

// Type is a method to get the type of a component.
func (r ActionsRow) Type() ComponentType {
	return ActionsRowComponent
}

// ButtonStyle is style of button.
type ButtonStyle uint

// Button styles.
const (
	// PrimaryButton is a button with blurple color.
	PrimaryButton ButtonStyle = 1
	// SecondaryButton is a button with grey color.
	SecondaryButton ButtonStyle = 2
	// SuccessButton is a button with green color.
	SuccessButton ButtonStyle = 3
	// DangerButton is a button with red color.
	DangerButton ButtonStyle = 4
	// LinkButton is a special type of button which navigates to a URL. Has grey color.
	LinkButton ButtonStyle = 5
)

// ComponentEmoji represents button emoji, if it does have one.
type ComponentEmoji struct {
	Name     string `json:"name,omitempty"`
	ID       string `json:"id,omitempty"`
	Animated bool   `json:"animated,omitempty"`
}

// Button represents button component.
type Button struct {
	Label    string         `json:"label"`
	Style    ButtonStyle    `json:"style"`
	Disabled bool           `json:"disabled"`
	Emoji    ComponentEmoji `json:"emoji"`

	// NOTE: Only button with LinkButton style can have link. Also, Link is mutually exclusive with CustomID.
	Link     string `json:"url,omitempty"`
	CustomID string `json:"custom_id,omitempty"`
}

// MarshalJSON is a method for marshaling Button to a JSON object.
func (b Button) MarshalJSON() ([]byte, error) {
	type button Button

	if b.Style == 0 {
		b.Style = PrimaryButton
	}

	return json.Marshal(struct {
		button
		Type ComponentType `json:"type"`
	}{
		button: button(b),
		Type:   b.Type(),
	})
}

// Type is a method to get the type of a component.
func (b Button) Type() ComponentType {
	return ButtonComponent
}

type SelectMenuOption struct {
	Label       string         `json:"label,omitempty"`
	Value       string         `json:"value"`
	Description string         `json:"description"`
	Emoji       ComponentEmoji `json:"emoji"`
	Default     bool           `json:"default"`
}

type SelectMenu struct {
	CustomID    string             `json:"custom_id,omitempty"`
	Placeholder string             `json:"placeholder"`
	MinValues   int                `json:"min_values,omitempty"`
	MaxValues   int                `json:"max_values,omitempty"`
	Options     []SelectMenuOption `json:"options"`
}

func (m SelectMenu) Type() ComponentType {
	return SelectMenuComponent
}

// MarshalJSON is a method for marshaling SelectMenu to a JSON object.
func (m SelectMenu) MarshalJSON() ([]byte, error) {
	type selectMenu SelectMenu

	return json.Marshal(struct {
		selectMenu
		Type ComponentType `json:"type"`
	}{
		selectMenu: selectMenu(m),
		Type:       m.Type(),
	})
}
