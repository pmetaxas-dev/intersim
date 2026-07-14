package main

import (
	"log"
	"net/http"
	"os"

	"intersim/internal/app"
)

// main loads application dependencies and starts the HTTP server.
func main() {
	questions, err := app.LoadQuestions("questions.json")
	if err != nil {
		log.Fatal(err)
	}

	application, err := app.New(app.Config{
		Questions:   questions,
		FallbackKey: os.Getenv("GROQ_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", application.Handler())
	mux.Handle("/", http.FileServer(http.Dir("./public")))

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Println("Server running on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
