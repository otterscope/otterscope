package store

import (
	"encoding/json"

	"github.com/otterscope/otterscope/internal/model"
)

// stepDetail is the JSON shape of the steps.detail column — kind-specific
// payloads too large/nested for flat columns.
type stepDetail struct {
	InputMessages  []model.Message `json:"inputMessages,omitempty"`
	OutputMessages []model.Message `json:"outputMessages,omitempty"`
	Arguments      string          `json:"arguments,omitempty"`
	Result         string          `json:"result,omitempty"`
}

func marshalDetail(st model.Step) string {
	var d stepDetail
	if st.LLM != nil {
		d.InputMessages = st.LLM.InputMessages
		d.OutputMessages = st.LLM.OutputMessages
	}
	if st.Tool != nil {
		d.Arguments = st.Tool.Arguments
		d.Result = st.Tool.Result
	}
	if d.InputMessages == nil && d.OutputMessages == nil && d.Arguments == "" && d.Result == "" {
		return ""
	}
	b, err := json.Marshal(d)
	if err != nil {
		return ""
	}
	return string(b)
}

func applyDetail(st *model.Step, raw string) {
	if raw == "" {
		return
	}
	var d stepDetail
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return
	}
	if st.LLM != nil {
		st.LLM.InputMessages = d.InputMessages
		st.LLM.OutputMessages = d.OutputMessages
	}
	if st.Tool != nil {
		st.Tool.Arguments = d.Arguments
		st.Tool.Result = d.Result
	}
}
