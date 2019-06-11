package app

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type server struct {
	router  *mux.Router
	backend Backend
}

func NewServer(backend Backend) *server {
	r := mux.NewRouter()
	s := &server{
		router:  r,
		backend: backend,
	}
	s.routes()
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *server) routes() {
	s.router.HandleFunc("/locks/{key}", s.AcquireHandler()).
		Methods("PUT", "POST")
	s.router.HandleFunc("/locks/{key}/{fence}", s.UpdateValueHandler()).
		Methods("PUT", "POST")
	s.router.HandleFunc("/locks/{key}/{fence}/heartbeat", s.HeartbeatHandler()).
		Methods("POST")
	s.router.HandleFunc("/locks/{key}", s.ReleaseHandler()).
		Methods("DELETE")
}

func (s *server) handleIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello world!"))
	}
}

func (s *server) AcquireHandler() http.HandlerFunc {
	/*
		type request struct {
			Nonce string
		}
	*/

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "application/json")

		vars := mux.Vars(r)
		key := vars["key"]

		if key == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"key missing"}`))
			return
		}

		nonce := r.URL.Query().Get("nonce")
		if nonce == "" {
			nonce = r.PostFormValue("nonce")
		}
		if nonce == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"nonce missing or empty"}`))
			return
		}
		if len(nonce) > 64 {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"nonce longer than 64 bytes"}`))
		}

		acq, err := s.backend.Acquire(key, nonce, leaseDuration)
		if err != nil {
			if _, ok := err.(ExpectedError); ok {
				w.WriteHeader(403)
			} else {
				w.WriteHeader(500)
			}
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(acq)
	}
}

func (s *server) UpdateValueHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("content-type", "application/json")

		vars := mux.Vars(r)
		key := vars["key"]
		if key == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"key missing or empty"}`))
			return
		}

		fenceStr := vars["fence"]
		if fenceStr == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"fence missing or empty"}`))
			return
		}

		fence, err := strconv.ParseInt(fenceStr, 10, 64)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"fence not an integer"}`))
			return
		}

		valueBytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"could not read request body"}`))
			return
		}

		err = s.backend.UpdateValue(key, fence, leaseDuration, string(valueBytes))
		if err != nil {
			if _, ok := err.(ExpectedError); ok {
				w.WriteHeader(403)
			} else {
				w.WriteHeader(500)
			}
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}
		w.WriteHeader(200)
	}
}

func (s *server) HeartbeatHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		key := vars["key"]
		if key == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"key missing or empty"}`))
			return
		}

		fenceStr := vars["fence"]
		if fenceStr == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"fence missing or empty"}`))
			return
		}

		fence, err := strconv.ParseInt(fenceStr, 10, 64)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"fence not an integer"}`))
			return
		}

		err = s.backend.Heartbeat(key, fence, leaseDuration)
		if err != nil {
			if _, ok := err.(ExpectedError); ok {
				w.WriteHeader(403)
			} else {
				w.WriteHeader(500)
			}
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}
		w.WriteHeader(200)
	}
}

func (s *server) ReleaseHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		key := vars["key"]
		if key == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"key missing or empty"}`))
			return
		}

		fenceStr := vars["fence"]
		if fenceStr == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"fence missing or empty"}`))
			return
		}

		fence, err := strconv.ParseInt(fenceStr, 10, 64)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"fence not an integer"}`))
			return
		}

		err = s.backend.Release(key, fence)
		if err != nil {
			if _, ok := err.(ExpectedError); ok {
				w.WriteHeader(403)
			} else {
				w.WriteHeader(500)
			}
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}
		w.WriteHeader(200)
	}
}
