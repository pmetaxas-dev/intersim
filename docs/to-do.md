# Version 0.8

- [x] Create `PRD.md`. **DONE**
- [x] Return to the JSON question pool after each follow-up. **DONE**
- [x] Ask for a Groq API key on the setup page when no environment fallback is
  configured; keep submitted keys in memory only. **DONE**
- [x] Populate `README.md`. **DONE**
- [x] Add automated tests under `/tests`. **DONE**

## Later update (1.0)

- [ ] Make the Interview Universal, but ommiting the questions.json (maybe keep it for fallback)
      At the start of the session, there will be a field with an interview description,
      so Groq will be creating 5 questions.
- [ ] There will be 3 Difficulty Levels.
- [ ] Ommit the fallback API KEy completely, handle only at front-end. Showing message "Please provide a valid API Key"

## Update 1.5

- [ ] Save every report in a file growth_reports.md.
- [ ] Use growth_reports.md to draw a chart of growth.

## Update 2.0

- [ ] Enable Memory using local files or maybe cloud SQL Database.
- [ ] As the memory becomes less entropic towards your knowledge and growth, there
      will be a prompt for proposing a book to read, that addresses as many of the
      "negatives" as possible.
