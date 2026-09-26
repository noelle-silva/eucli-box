package gateway

import (
	"net/http"

	"eucli-box/pkg/types"
)

func (s *system) handleLoadRequestRecordConfig(w http.ResponseWriter, r *http.Request) {
	config, err := s.requestRecords.LoadRequestRecordConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, config)
}

func (s *system) handleSaveRequestRecordConfig(w http.ResponseWriter, r *http.Request) {
	config, err := decodeJSON[types.RequestRecordConfig](r)
	if err != nil {
		writeError(w, err)
		return
	}
	saved, err := s.requestRecords.SaveRequestRecordConfig(r.Context(), config)
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, saved)
}

func (s *system) handleListRequestRecords(w http.ResponseWriter, r *http.Request) {
	records, err := s.requestRecords.ListRequestRecords(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, records)
}

func (s *system) handleLoadRequestRecord(w http.ResponseWriter, r *http.Request) {
	record, err := s.requestRecords.LoadRequestRecord(r.Context(), r.PathValue("recordID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, record)
}
