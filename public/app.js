let currentQuestionId = null;
let requiresApiKey = true;

// loadConfig checks whether the server already has an environment API key.
async function loadConfig() {
    const input = document.getElementById('api-key');
    const help = document.getElementById('api-key-help');

    try {
        const response = await fetch('/api/config', { cache: 'no-store' });
        if (!response.ok) throw new Error(await responseError(response));

        const config = await response.json();
        requiresApiKey = config.requiresApiKey;
        input.required = requiresApiKey;
        help.textContent = requiresApiKey
            ? 'Required. The key remains in server memory only for this interview.'
            : 'Optional. Leave blank to use the server GROQ_API_KEY.';
    } catch (error) {
        requiresApiKey = true;
        input.required = true;
        help.textContent = 'Configuration check failed. Enter an API key to continue.';
        console.error('Configuration preflight failed:', error);
    }
}

// startInterview starts a new interview with an optional browser-provided key.
async function startInterview() {
    const input = document.getElementById('api-key');
    const apiKey = input.value.trim();
    const button = document.querySelector('#setup-screen button');

    if (requiresApiKey && !apiKey) {
        input.focus();
        alert('Please enter a working Groq API key.');
        return;
    }

    button.disabled = true;
    try {
        const response = await fetch('/api/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ apiKey })
        });
        if (!response.ok) throw new Error(await responseError(response));

        const question = await response.json();
        input.value = '';
        document.getElementById('setup-screen').classList.add('hidden');
        document.getElementById('question-screen').classList.remove('hidden');
        displayQuestion(question);
    } catch (error) {
        alert(`Unable to start the interview: ${error.message}`);
        console.error('Failed to start interview:', error);
    } finally {
        button.disabled = false;
    }
}

// displayQuestion updates the workspace with the current question.
function displayQuestion(question) {
    if (!question) return;

    currentQuestionId = question.id;
    const category = question.category === 'Follow-up'
        ? '🔍 Follow-up Question'
        : question.category;
    document.getElementById('question-category').textContent = category;
    document.getElementById('question-text').textContent = question.question;
    document.getElementById('user-answer').value = '';
    document.getElementById('user-answer').focus();
}

// submitAnswer sends the current response and advances the interview.
async function submitAnswer() {
    const answerInput = document.getElementById('user-answer');
    const answer = answerInput.value.trim();
    const button = document.querySelector('#question-screen button');

    if (!answer) {
        alert('Please type an answer first.');
        return;
    }

    button.disabled = true;
    try {
        const response = await fetch('/api/answer', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ questionId: currentQuestionId, answer })
        });
        if (!response.ok) throw new Error(await responseError(response));

        const data = await response.json();
        appendFeedback(data.evaluation);
        if (data.isFinished) {
            await showFinalReport();
        } else {
            displayQuestion(data.nextQuestion);
        }
    } catch (error) {
        alert(`Unable to submit the answer: ${error.message}`);
        console.error('Failed to submit answer:', error);
    } finally {
        button.disabled = false;
    }
}

// appendFeedback safely renders an evaluation in the interview feed.
function appendFeedback(evaluation) {
    const block = document.createElement('div');
    block.className = 'feedback-block';

    const score = document.createElement('p');
    const scoreTag = document.createElement('span');
    scoreTag.className = 'score-tag';
    scoreTag.textContent = `Score: ${evaluation.score}/100`;
    score.appendChild(scoreTag);

    const pros = document.createElement('p');
    pros.textContent = `✓ Pros: ${evaluation.feedbackGood}`;
    const cons = document.createElement('p');
    cons.textContent = `✗ Cons: ${evaluation.feedbackBad}`;
    block.append(score, pros, cons);

    const chatLog = document.getElementById('chat-log');
    chatLog.appendChild(block);
    chatLog.scrollTop = chatLog.scrollHeight;
}

// showFinalReport requests and renders the completed interview report.
async function showFinalReport() {
    document.getElementById('question-screen').classList.add('hidden');
    document.getElementById('report-screen').classList.remove('hidden');

    try {
        const response = await fetch('/api/report', { cache: 'no-store' });
        if (!response.ok) throw new Error(await responseError(response));

        const report = await response.json();
        document.getElementById('final-score').textContent = `${report.finalScore}%`;
        renderList('weaknesses-list', report.weaknesses);
        renderList('suggestions-list', report.studySuggestions);
    } catch (error) {
        alert(`Unable to load the final report: ${error.message}`);
        console.error('Failed to load report:', error);
    }
}

function renderList(elementId, items) {
    const list = document.getElementById(elementId);
    list.replaceChildren(...items.map(item => {
        const entry = document.createElement('li');
        entry.textContent = item;
        return entry;
    }));
}

async function responseError(response) {
    try {
        const payload = await response.json();
        return payload.error || `Server returned HTTP ${response.status}`;
    } catch {
        return `Server returned HTTP ${response.status}`;
    }
}

document.addEventListener('DOMContentLoaded', loadConfig);
