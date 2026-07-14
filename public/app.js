let currentQuestionId = null;

async function startInterview() {
    try {
        console.log("Sending request to /api/start...");
        const response = await fetch('/api/start');
        
        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`Server error: ${response.status} - ${errorText}`);
        }
        
        const question = await response.json();
        console.log("Question received successfully:", question);
        
        document.getElementById('setup-screen').classList.add('hidden');
        document.getElementById('question-screen').classList.remove('hidden');
        
        displayQuestion(question);
    } catch (error) {
        console.error("Failed to start interview:", error);
        alert("Σφάλμα κατά την εκκίνηση: " + error.message);
    }
}

function displayQuestion(question) {
    if (!question) return;
    
    currentQuestionId = question.id;
    
    // Αν η κατηγορία είναι "Follow-up", πρόσθεσε μια ένδειξη
    let displayCategory = question.category;
    if (question.category === "Follow-up") {
        displayCategory = "🔍 Follow-up Question";
    }
    
    document.getElementById('question-category').innerText = displayCategory;
    document.getElementById('question-text').innerText = question.question;
    document.getElementById('user-answer').value = '';
}

async function submitAnswer() {
    const answer = document.getElementById('user-answer').value;
    if (!answer.trim()) return alert("Please type an answer first.");

    const chatLog = document.getElementById('chat-log');
    
    const response = await fetch('/api/answer', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ questionId: currentQuestionId, answer: answer })
    });
    
   const data = await response.json();
    
    const evalBlock = document.createElement('div');
    evalBlock.className = 'feedback-block';
    
    // Αφαιρέσαμε το followUpHtml από εδώ για να μην φαίνεται στα αριστερά
    evalBlock.innerHTML = `
        <p><span class="score-tag">Score: ${data.evaluation.score}/100</span></p>
        <p><strong>✓ Pros:</strong> ${data.evaluation.feedbackGood}</p>
        <p><strong>✗ Cons:</strong> ${data.evaluation.feedbackBad}</p>
    `;
    
    chatLog.appendChild(evalBlock);
    chatLog.scrollTop = chatLog.scrollHeight;

    if (data.isFinished) {
        showFinalReport();
    } else {
        // ΕΔΩ ΗΤΑΝ ΤΟ ΛΑΘΟΣ: Πρέπει να στείλεις το αντικείμενο που ήρθε από το API
        displayQuestion(data.nextQuestion); 
    }
}

async function showFinalReport() {
    document.getElementById('question-screen').classList.add('hidden');
    document.getElementById('report-screen').classList.remove('hidden');

    try {
        const response = await fetch('/api/report');
        const report = await response.json();

        // Ενημέρωση του Score (αυτό έλειπε)
        const scoreEl = document.getElementById('final-score');
        if (scoreEl) {
            scoreEl.innerText = `${report.finalScore}%`;
        }

        // Ενημέρωση Weaknesses
        const weaknessesUl = document.getElementById('weaknesses-list');
        weaknessesUl.innerHTML = report.weaknesses.map(w => `<li>${w}</li>`).join('');

        // Ενημέρωση Suggestions
        const suggestionsUl = document.getElementById('suggestions-list');
        suggestionsUl.innerHTML = report.studySuggestions.map(s => `<li>${s}</li>`).join('');
        
    } catch (err) {
        console.error("Error loading report:", err);
    }
}