package tuiapp

import "fmt"

func ValidateState(s State) error {
	if err := validateCursor("menu", s.Menu.Cursor, len(s.Menu.Items)); err != nil {
		return err
	}
	if err := validateCursor("options", s.Options.Cursor, len(s.Options.Items)); err != nil {
		return err
	}
	if err := validateCursor("select", s.Select.Cursor, len(s.Select.Items)); err != nil {
		return err
	}
	if err := validateCursor("job_form", s.JobForm.Cursor, 7); err != nil {
		return err
	}
	if err := validateCursor("api_key", s.APIKey.Cursor, len(s.APIKey.Items)); err != nil {
		return err
	}
	if s.Screen == ScreenMenu && s.Pending.Kind != RequestNone {
		return fmt.Errorf("menu cannot hold pending request: %#v", s.Pending)
	}
	if s.Screen == ScreenResult && s.Pending.Kind != RequestNone {
		return fmt.Errorf("result cannot hold pending request: %#v", s.Pending)
	}
	if s.Running.CancelPending && s.Pending.Kind != RequestTranscription {
		return fmt.Errorf("cancel pending requires transcription request")
	}
	if s.Pending.Kind == RequestNone && s.Pending.ID != "" {
		return fmt.Errorf("request ID without request kind")
	}
	if s.Screen == ScreenEdit && s.APIKey.PendingMethod == "" && s.Edit.Field == "" {
		return fmt.Errorf("edit screen requires a pending api key method or edit field")
	}
	if s.APIKey.PendingMethod == "" && s.APIKey.PendingValue != "" {
		return fmt.Errorf("api key pending value requires a pending method")
	}
	return nil
}

func validateCursor(name string, cursor, length int) error {
	if length == 0 {
		if cursor != 0 {
			return fmt.Errorf("%s cursor = %d, want 0 for empty list", name, cursor)
		}
		return nil
	}
	if cursor < 0 || cursor >= length {
		return fmt.Errorf("%s cursor = %d, length = %d", name, cursor, length)
	}
	return nil
}
