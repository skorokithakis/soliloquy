// Vote without a page reload. The forms still work as plain POSTs when
// JavaScript is unavailable. A failed request reloads the page instead of
// resubmitting, because the vote may already have been committed.
document.addEventListener("submit", async (event) => {
	const form = event.target;
	if (!(form instanceof HTMLFormElement) || !form.classList.contains("vote-form")) {
		return;
	}

	event.preventDefault();

	try {
		const response = await fetch(form.action, {
			method: "POST",
			headers: { Accept: "application/json" },
			credentials: "same-origin",
		});
		if (!response.ok) {
			window.location.reload();
			return;
		}

		const data = await response.json();
		const item = form.closest(".thread, .comment");
		const score = item && item.querySelector(".score");
		if (score) {
			score.textContent = data.score_text;
		}

		const button = form.querySelector("button");
		if (button) {
			button.textContent = data.button_text;
			button.dataset.voted = data.voted ? "true" : "false";
		}
	} catch (error) {
		window.location.reload();
	}
});

// Live badge and comment updates over the page's single SSE stream.
const eventsURL = document.body.dataset.sseUrl;
if (eventsURL && "EventSource" in window) {
	const stream = new EventSource(eventsURL);

	stream.addEventListener("badge", (event) => {
		const data = JSON.parse(event.data);
		const link = document.querySelector(".notifications-link");
		if (!link) {
			return;
		}

		let badge = link.querySelector(".badge");
		if (!data.count) {
			if (badge) {
				badge.remove();
			}
			return;
		}

		if (!badge) {
			badge = document.createElement("span");
			badge.className = "badge";
			link.appendChild(badge);
		}
		badge.textContent = data.count;
	});

	stream.addEventListener("comment", (event) => {
		const data = JSON.parse(event.data);
		if (document.getElementById("c" + data.comment_id)) {
			// The author's page already has this comment after its redirect.
			return;
		}

		const selector = data.parent_id === null
			? "#comments"
			: 'ol.comment-replies[data-parent-id="' + data.parent_id + '"]';
		const container = document.querySelector(selector);
		if (!container) {
			return;
		}

		container.insertAdjacentHTML("beforeend", data.html);

		const empty = document.querySelector(".no-comments");
		if (empty) {
			empty.remove();
		}
	});
}
