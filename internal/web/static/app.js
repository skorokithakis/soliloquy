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
		const score = scoreElement(form);
		if (score) {
			score.element.textContent = score.format === "number" ? data.score : data.score_text;
			score.element.classList.toggle("on", Boolean(data.voted));
		}

		const button = form.querySelector("button");
		if (button) {
			button.classList.toggle("on", Boolean(data.voted));
			button.dataset.voted = data.voted ? "true" : "false";
			button.setAttribute("aria-pressed", data.voted ? "true" : "false");
			button.setAttribute("aria-label", data.voted ? "Remove upvote" : "Upvote");
		}
	} catch (error) {
		window.location.reload();
	}
});

// scoreElement finds the score shown next to a vote button. Front-page rows
// use the compact .sc column, while thread headers and comments use .score.
function scoreElement(form) {
	const row = form.closest(".row");
	if (row) {
		const element = row.querySelector(".sc");
		return element ? { element, format: "number" } : null;
	}

	const item = form.closest(".thread, .comment, .c");
	const element = item && item.querySelector(".score");
	if (!element) {
		return null;
	}
	return {
		element,
		format: element.dataset.scoreFormat === "number" ? "number" : "points",
	};
}

// Update the character counter for a field. Comment editors find their counter
// inside .editor; the thread form names its counters because they live outside
// the editor box.
function updateCounter(field) {
	const form = field.closest("form") || document;
	const name = field.dataset.counterName;
	let counter = null;
	if (name) {
		counter = form.querySelector('[data-counter="' + name + '"]');
	} else {
		const editor = field.closest(".editor");
		counter = editor && editor.querySelector("[data-counter]");
	}
	if (!counter) {
		return;
	}

	const max = field.maxLength > 0 ? field.maxLength : 20000;
	counter.textContent = Array.from(field.value).length + " / " + max;
}

document.addEventListener("input", (event) => {
	if (event.target instanceof HTMLTextAreaElement || event.target instanceof HTMLInputElement) {
		updateCounter(event.target);
	}
});

// Submit the form with Ctrl+Enter (Cmd+Enter on macOS) from any multiline box.
// requestSubmit runs validation and the submit handlers, like the button does.
document.addEventListener("keydown", (event) => {
	if (event.key !== "Enter" || !(event.ctrlKey || event.metaKey) || event.repeat || event.isComposing) {
		return;
	}
	const field = event.target;
	if (!(field instanceof HTMLTextAreaElement) || !field.form) {
		return;
	}
	event.preventDefault();
	field.form.requestSubmit();
});

// Collapse and expand comment subtrees. The controls are hidden in the HTML
// and only enabled here, so they do nothing when JavaScript is unavailable.
document.addEventListener("click", (event) => {
	const toggle = event.target.closest(".tog");
	if (!toggle) {
		return;
	}

	const comment = toggle.closest(".c");
	if (!comment) {
		return;
	}

	const collapsed = comment.classList.toggle("collapsed");
	toggle.textContent = collapsed ? "[+]" : "[-]";
	toggle.setAttribute("aria-expanded", collapsed ? "false" : "true");
	toggle.setAttribute("aria-label", collapsed ? "Expand comment" : "Collapse comment");
});

// Open the reply form inline. Without JavaScript the reply link goes to the
// standalone reply page instead.
document.addEventListener("click", (event) => {
	const link = event.target.closest("a.reply-link");
	if (!link || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
		return;
	}

	const comment = link.closest(".c");
	if (!comment) {
		return;
	}

	const existing = Array.from(comment.children).find((child) => child.classList.contains("reply-form"));
	if (existing) {
		event.preventDefault();
		const textarea = existing.querySelector("textarea");
		if (textarea) {
			textarea.focus();
		}
		return;
	}

	const actions = Array.from(comment.children).find((child) => child.classList.contains("ca"));
	if (!actions) {
		return;
	}

	event.preventDefault();

	const header = Array.from(comment.children).find((child) => child.classList.contains("ch"));
	const authorElement = header && header.querySelector(".u, .gone");
	const author = authorElement ? authorElement.textContent : "this comment";

	const form = document.createElement("form");
	form.className = "reply-form";
	form.method = "post";
	form.action = link.getAttribute("href");
	form.setAttribute("aria-label", "Reply to " + author);

	const editor = document.createElement("div");
	editor.className = "editor";

	const textarea = document.createElement("textarea");
	textarea.name = "body";
	textarea.rows = 6;
	textarea.maxLength = 20000;
	textarea.required = true;
	textarea.placeholder = "Reply to " + author;
	textarea.setAttribute("aria-label", "Reply");

	const bar = document.createElement("div");
	bar.className = "edbar";

	const note = document.createElement("span");
	note.textContent = "replying to " + author;

	const spacer = document.createElement("span");
	spacer.className = "sp";

	const counter = document.createElement("span");
	counter.dataset.counter = "";
	counter.textContent = "0 / 20000";

	const cancel = document.createElement("button");
	cancel.type = "button";
	cancel.className = "btn ghost";
	cancel.textContent = "cancel";
	cancel.addEventListener("click", (cancelEvent) => {
		cancelEvent.preventDefault();
		form.remove();
	});

	const submit = document.createElement("button");
	submit.type = "submit";
	submit.className = "btn pri";
	submit.textContent = "reply";

	bar.append(note, spacer, counter, cancel, submit);
	editor.append(textarea, bar);
	form.append(editor);
	actions.insertAdjacentElement("afterend", form);

	updateCounter(textarea);
	textarea.focus();
});

// Enable the controls that need JavaScript on every existing comment.
function enhanceComments(root) {
	root.querySelectorAll(".tog[hidden]").forEach((toggle) => {
		toggle.hidden = false;
	});
	root.querySelectorAll("textarea, input[data-counter-name]").forEach(updateCounter);
}

enhanceComments(document);

// Copy-to-clipboard buttons. The URL is already visible and selectable when
// JavaScript or clipboard access is unavailable.
document.addEventListener("click", (event) => {
	const button = event.target.closest("[data-copy]");
	if (!button || !navigator.clipboard) {
		return;
	}

	navigator.clipboard.writeText(button.dataset.copy).then(() => {
		const original = button.textContent;
		button.textContent = "copied";
		window.setTimeout(() => {
			button.textContent = original;
		}, 1200);
	}).catch(() => {
		// Leave the text selectable; nothing else to do.
	});
});

// Filter the admin user table in the browser. Without JavaScript every row is
// visible.
const userFilter = document.querySelector("[data-user-filter]");
if (userFilter) {
	userFilter.addEventListener("input", () => {
		const query = userFilter.value.trim().toLowerCase();
		document.querySelectorAll("tr[data-username]").forEach((row) => {
			row.hidden = query !== "" && !row.dataset.username.includes(query);
		});
	});
}

// The delete-account button is enabled without JavaScript so the server can
// validate the username. With JavaScript, stop an obvious mismatch early.
const confirmUsername = document.querySelector("[data-confirm-username]");
const deleteAccount = document.querySelector("[data-delete-account]");
if (confirmUsername && deleteAccount) {
	const expected = confirmUsername.dataset.confirmUsername.toLowerCase();
	const updateDeleteButton = () => {
		deleteAccount.disabled = confirmUsername.value.trim().toLowerCase() !== expected;
	};
	confirmUsername.addEventListener("input", updateDeleteButton);
	updateDeleteButton();
}

// Login throttle: count down the wait shown by the server, then let the user
// try again. The server renders the button disabled so the wait is clear even
// without JavaScript.
const waitButton = document.querySelector("[data-wait-seconds]");
if (waitButton) {
	let remaining = Number(waitButton.dataset.waitSeconds) || 0;
	const tick = () => {
		if (remaining <= 0) {
			waitButton.disabled = false;
			waitButton.textContent = "log in";
			return;
		}
		waitButton.textContent = "wait " + remaining + "s";
		remaining--;
		window.setTimeout(tick, 1000);
	};
	tick();
}

// Live username checks on the invite sign-up page. The static rules remain
// visible without JavaScript and the server validates the submitted name.
const usernameInput = document.querySelector("[data-username-check]");
if (usernameInput) {
	const lengthCheck = document.querySelector('[data-check="length"]');
	const charsCheck = document.querySelector('[data-check="chars"]');
	const availableCheck = document.querySelector('[data-check="available"]');
	let checkTimer = 0;
	let checkRequest = 0;

	const setCheck = (element, state, text) => {
		if (!element) {
			return;
		}
		element.classList.toggle("ok", state === "ok");
		element.classList.toggle("bad", state === "bad");
		element.textContent = (state === "ok" ? "✓ " : state === "bad" ? "! " : "· ") + text;
	};

	const updateUsernameChecks = () => {
		const value = usernameInput.value.trim().toLowerCase();
		const lengthOK = value.length >= 3 && value.length <= 20;
		const charsOK = /^[a-z0-9_-]+$/.test(value);

		setCheck(lengthCheck, lengthOK ? "ok" : "", "3 to 20 characters");
		setCheck(charsCheck, charsOK ? "ok" : "", "only a-z, 0-9, _ and -");
		if (!lengthOK || !charsOK) {
			setCheck(availableCheck, "", "available");
			return;
		}

		window.clearTimeout(checkTimer);
		checkTimer = window.setTimeout(async () => {
			const request = ++checkRequest;
			try {
				const response = await fetch(usernameInput.dataset.checkUrl + "?username=" + encodeURIComponent(value), {
					headers: { Accept: "application/json" },
					credentials: "same-origin",
				});
				if (!response.ok || request !== checkRequest) {
					return;
				}
				const data = await response.json();
				if (request !== checkRequest) {
					return;
				}
				if (!data.valid) {
					setCheck(availableCheck, "", "available");
				} else if (data.available) {
					setCheck(availableCheck, "ok", "available");
				} else {
					setCheck(availableCheck, "bad", "taken");
				}
			} catch (error) {
				// Leave the last state; the server checks the name on submit.
			}
		}, 300);
	};

	usernameInput.addEventListener("input", updateUsernameChecks);
	if (usernameInput.value.trim() !== "") {
		updateUsernameChecks();
	}
}

// Thread form: show the write/preview tabs and keep the preview and mention
// notification line in sync with the body. Without this script the tabs and
// preview stay hidden and the textarea works normally.
const threadForm = document.querySelector(".thread-form");
if (threadForm) {
	const tabs = threadForm.querySelector("[data-tabs]");
	const previewPane = threadForm.querySelector("[data-preview]");
	const textarea = threadForm.querySelector('textarea[name="body"]');
	const mentionNote = threadForm.querySelector("[data-mention-note]");
	const writeTab = tabs && tabs.querySelector('[data-tab="write"]');
	const previewTab = tabs && tabs.querySelector('[data-tab="preview"]');

	if (tabs && previewPane && textarea) {
		tabs.hidden = false;

		let previewRequest = 0;
		const requestPreview = async () => {
			const request = ++previewRequest;
			try {
				const response = await fetch("/preview", {
					method: "POST",
					headers: {
						Accept: "application/json",
						"Content-Type": "application/x-www-form-urlencoded",
					},
					credentials: "same-origin",
					body: new URLSearchParams({ body: textarea.value }),
				});
				if (!response.ok || request !== previewRequest) {
					return;
				}

				const data = await response.json();
				if (request !== previewRequest) {
					return;
				}
				previewPane.innerHTML = data.html;
				if (mentionNote) {
					const names = (data.mentions || []).map((name) => "@" + name);
					if (names.length === 0) {
						mentionNote.textContent = "";
					} else {
						mentionNote.textContent = names.join(", ") +
							(names.length === 1 ? " will get a notification" : " will get notifications");
					}
				}
			} catch (error) {
				// Keep the last good preview; the plain form still works.
			}
		};

		const showTab = (name) => {
			const showPreview = name === "preview";
			if (writeTab) {
				writeTab.classList.toggle("on", !showPreview);
				writeTab.setAttribute("aria-selected", String(!showPreview));
			}
			if (previewTab) {
				previewTab.classList.toggle("on", showPreview);
				previewTab.setAttribute("aria-selected", String(showPreview));
			}
			textarea.hidden = showPreview;
			previewPane.hidden = !showPreview;
			if (showPreview) {
				requestPreview();
			}
		};

		tabs.addEventListener("click", (event) => {
			const button = event.target.closest("[data-tab]");
			if (button) {
				showTab(button.dataset.tab);
			}
		});

		let previewTimer = 0;
		textarea.addEventListener("input", () => {
			window.clearTimeout(previewTimer);
			previewTimer = window.setTimeout(requestPreview, 400);
		});

		if (textarea.value.trim() !== "") {
			requestPreview();
		}
	}
}

// Live badge and comment updates over the page's single SSE stream.
const eventsURL = document.body.dataset.sseUrl;
if (eventsURL && "EventSource" in window) {
	const stream = new EventSource(eventsURL);
	const liveIndicator = document.querySelector(".live[data-live]");

	stream.addEventListener("open", () => {
		if (liveIndicator) {
			liveIndicator.hidden = false;
		}
	});

	stream.addEventListener("error", () => {
		if (liveIndicator) {
			liveIndicator.hidden = true;
		}
	});

	stream.addEventListener("badge", (event) => {
		const data = JSON.parse(event.data);
		const link = document.querySelector(".notifications-link");
		if (!link) {
			return;
		}

		const count = Number(data.count) || 0;
		link.setAttribute("aria-label", count ? "Notifications, " + count + " unread" : "Notifications");

		let badge = link.querySelector(".badge");
		if (!count) {
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
		badge.textContent = count;
	});

	stream.addEventListener("comment", (event) => {
		const data = JSON.parse(event.data);
		if (document.getElementById("c" + data.comment_id)) {
			// The author's page already has this comment after its redirect.
			return;
		}

		const selector = data.parent_id === null
			? "#comments"
			: 'ol.kids[data-parent-id="' + data.parent_id + '"]';
		const container = document.querySelector(selector);
		if (!container) {
			return;
		}

		container.insertAdjacentHTML("beforeend", data.html);

		const inserted = document.getElementById("c" + data.comment_id);
		if (inserted) {
			inserted.classList.add("new");
			const header = Array.from(inserted.children).find((child) => child.classList.contains("ch"));
			const toggle = header && header.querySelector(".tog");
			if (header) {
				const tag = document.createElement("span");
				tag.className = "tag";
				tag.textContent = "new";
				header.insertBefore(tag, toggle);
			}
			enhanceComments(inserted);
		}

		const empty = document.querySelector(".no-comments");
		if (empty) {
			empty.remove();
		}
	});
}
