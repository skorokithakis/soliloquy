// Ask before submitting a form whose data-confirm attribute holds the
// question. The listener is on the document so forms inserted later, such as
// comment delete forms in live updates, are covered too. Without JavaScript
// the forms submit without a question.
document.addEventListener("submit", (event) => {
	const form = event.target;
	if (!(form instanceof HTMLFormElement) || !form.dataset.confirm) {
		return;
	}

	if (!window.confirm(form.dataset.confirm)) {
		event.preventDefault();
	}
});

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

	const previewPane = document.createElement("div");
	previewPane.className = "preview md";
	previewPane.hidden = true;
	previewPane.dataset.preview = "";

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

	bar.append(note, spacer, counter, submit, cancel);
	editor.append(createPreviewTabs(), textarea, previewPane, bar);
	form.append(editor);
	actions.insertAdjacentElement("afterend", form);

	setupEditorPreview(form);
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

// Account page: picking a theme changes the page at once and saves the choice
// in the background. The form still submits normally when JavaScript is
// unavailable, so the save button is only hidden once this script runs.
const themeSave = document.querySelector("[data-theme-save]");
const themeForm = themeSave && themeSave.form;
if (themeForm) {
	themeSave.hidden = true;

	// Only one save may run at a time, because two requests could reach the
	// server out of order and the older choice would win. Changes made while a
	// save runs are coalesced into one more save with the latest selection.
	let saving = false;
	let queued = false;

	const saveTheme = async () => {
		if (saving) {
			queued = true;
			return;
		}

		saving = true;
		for (;;) {
			queued = false;
			try {
				const response = await fetch(themeForm.action, {
					method: "POST",
					headers: {
						Accept: "text/html",
						"Content-Type": "application/x-www-form-urlencoded",
					},
					credentials: "same-origin",
					body: new URLSearchParams(new FormData(themeForm)),
				});
				if (!response.ok) {
					throw new Error("theme save failed");
				}
			} catch (error) {
				saving = false;
				// The plain form submit saves the choice and reports errors.
				themeForm.submit();
				return;
			}

			if (!queued) {
				saving = false;
				return;
			}
		}
	};

	const themeRadios = themeForm.querySelectorAll('input[name="theme"]');
	themeRadios.forEach((radio) => {
		radio.addEventListener("change", () => {
			if (!radio.checked) {
				return;
			}

			const root = document.documentElement;
			root.classList.remove("auto", "light", "dark");
			root.classList.add(radio.value);
			saveTheme();
		});
	});
}

// Account page: keep the section menu in step with the section being read.
// Without this script the link marked in the HTML stays marked.
const accountNav = document.querySelector(".account-page .toc");
if (accountNav) {
	const sections = Array.from(accountNav.querySelectorAll('a[href^="#"]'))
		.map((link) => ({ link, section: document.getElementById(link.hash.slice(1)) }))
		.filter((entry) => entry.section);

	const markCurrent = (current) => {
		sections.forEach((entry) => {
			if (entry === current) {
				entry.link.setAttribute("aria-current", "true");
			} else {
				entry.link.removeAttribute("aria-current");
			}
		});
	};

	// The section under the reading line near the top is the current one; the
	// last section may be too short to reach the line, so the end of the page
	// selects it. A click wins for a moment, because the browser's scroll to a
	// section near the end of the page can land at the bottom.
	let clicked = 0;
	const updateCurrent = () => {
		if (clicked || sections.length === 0) {
			return;
		}
		let current = sections[0];
		for (const entry of sections) {
			if (entry.section.getBoundingClientRect().top <= 120) {
				current = entry;
			}
		}
		const atEnd = window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - 2;
		markCurrent(atEnd ? sections[sections.length - 1] : current);
	};

	window.addEventListener("scroll", updateCurrent, { passive: true });
	window.addEventListener("resize", updateCurrent);
	sections.forEach((entry) => {
		entry.link.addEventListener("click", () => {
			markCurrent(entry);
			window.clearTimeout(clicked);
			clicked = window.setTimeout(() => {
				clicked = 0;
			}, 300);
		});
	});
	updateCurrent();
}

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

// setupEditorPreview shows the write/preview tabs on a Markdown editor and
// keeps the preview in sync with the textarea. The tab bar and preview pane
// are hidden in the HTML, so without this script the textarea works normally.
// The optional mentionNote shows who would get a notification, which the thread
// form uses but comment forms do not.
function setupEditorPreview(form, mentionNote) {
	const tabs = form.querySelector("[data-tabs]");
	const previewPane = form.querySelector("[data-preview]");
	const textarea = form.querySelector('textarea[name="body"]');
	if (!tabs || !previewPane || !textarea) {
		return;
	}

	const writeTab = tabs.querySelector('[data-tab="write"]');
	const previewTab = tabs.querySelector('[data-tab="preview"]');
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
		if (showPreview) {
			// Match the textarea it replaces so the form does not jump. Measure
			// before hiding it, because a hidden element has no height.
			previewPane.style.minHeight = textarea.offsetHeight + "px";
		}
		textarea.hidden = showPreview;
		previewPane.hidden = !showPreview;
		if (showPreview) {
			requestPreview();
		}
	};

	// A hidden required textarea cannot show the browser's validation message,
	// so go back to the write tab when a submit fails validation.
	textarea.addEventListener("invalid", () => {
		showTab("write");
	});

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

// createPreviewTabs builds the tab bar for a comment form created by the
// script. It matches the markup the templates use.
function createPreviewTabs() {
	const tabs = document.createElement("div");
	tabs.className = "tabs";
	tabs.setAttribute("role", "tablist");
	tabs.hidden = true;
	tabs.dataset.tabs = "";

	const writeTab = document.createElement("button");
	writeTab.type = "button";
	writeTab.className = "on";
	writeTab.setAttribute("role", "tab");
	writeTab.setAttribute("aria-selected", "true");
	writeTab.dataset.tab = "write";
	writeTab.textContent = "write";

	const previewTab = document.createElement("button");
	previewTab.type = "button";
	previewTab.setAttribute("role", "tab");
	previewTab.setAttribute("aria-selected", "false");
	previewTab.dataset.tab = "preview";
	previewTab.textContent = "preview";

	tabs.append(writeTab, previewTab);
	return tabs;
}

// Add the write/preview tabs to every comment form the page rendered. The
// inline reply form is set up in its click handler once it exists.
document.querySelectorAll("[data-tabs]").forEach((tabs) => {
	const form = tabs.closest("form");
	if (form) {
		setupEditorPreview(form, form.querySelector("[data-mention-note]"));
	}
});

const threadForm = document.querySelector(".thread-form");
if (threadForm) {
	const titleInput = threadForm.querySelector('input[name="title"]');
	const urlInput = threadForm.querySelector('input[name="url"]');
	const titleStatus = threadForm.querySelector("[data-title-status]");

	// Fetch the page title when someone pastes a URL or leaves the field. The
	// change event is the fallback for browsers whose paste event fires before
	// the input value updates, and for a typed URL. A plain form still works.
	if (titleInput && urlInput) {
		let titleRequest = 0;
		// The URL being fetched now. A paste is followed by a change event on
		// blur, so this stops a second fetch of the same URL while the first runs.
		let pendingURL = "";

		const setTitleStatus = (text) => {
			if (titleStatus) {
				titleStatus.textContent = text;
			}
		};

		const requestTitle = async () => {
			const url = urlInput.value.trim();
			if (url !== "" && url === pendingURL) {
				return;
			}
			const request = ++titleRequest;
			pendingURL = "";
			if (url === "" || titleInput.value !== "") {
				setTitleStatus("");
				return;
			}
			pendingURL = url;

			setTitleStatus("fetching title…");
			try {
				const response = await fetch("/title", {
					method: "POST",
					headers: {
						Accept: "application/json",
						"Content-Type": "application/x-www-form-urlencoded",
					},
					credentials: "same-origin",
					body: new URLSearchParams({ url }),
				});
				const data = response.ok ? await response.json() : {};
				// Fill only if this is still the latest request, the URL has not
				// changed since, and the member has not typed a title.
				if (
					request === titleRequest &&
					urlInput.value.trim() === url &&
					titleInput.value === "" &&
					data.title
				) {
					titleInput.value = data.title;
					updateCounter(titleInput);
				}
			} catch (error) {
				// The title is a convenience; the plain form still works.
			} finally {
				if (request === titleRequest) {
					pendingURL = "";
					setTitleStatus("");
				}
			}
		};

		urlInput.addEventListener("paste", () => {
			// The paste event fires before the input value updates.
			window.setTimeout(requestTitle, 0);
		});
		urlInput.addEventListener("change", requestTitle);
	}
}

// Desktop alerts for personal notifications. The preference is per browser,
// so it lives in localStorage rather than the account record.
const desktopAlertsKey = "desktop-alerts";

function desktopAlertsEnabled() {
	try {
		return window.localStorage.getItem(desktopAlertsKey) === "on";
	} catch (error) {
		return false;
	}
}

function setDesktopAlertsEnabled(enabled) {
	try {
		if (enabled) {
			window.localStorage.setItem(desktopAlertsKey, "on");
		} else {
			window.localStorage.removeItem(desktopAlertsKey);
		}
	} catch (error) {
		// Storage is unavailable; leave alerts off.
	}
}

// desktopAlertsActive is the effective state: the browser preference is on
// and the browser still grants permission. Stored "on" alone is not enough,
// because permission can return to "default", and the button should then ask
// for it again instead of switching alerts off.
function desktopAlertsActive() {
	return (
		desktopAlertsEnabled() &&
		"Notification" in window &&
		Notification.permission === "granted"
	);
}

// Account page: the button requests permission from the click, because
// browsers only show the prompt for a user gesture.
const desktopAlertsButton = document.querySelector("[data-desktop-alerts]");
const desktopAlertsStatus = document.querySelector("[data-desktop-alerts-status]");
if (desktopAlertsButton && desktopAlertsStatus) {
	const supported = "Notification" in window;

	const updateDesktopAlerts = () => {
		const enabled = desktopAlertsActive();
		desktopAlertsStatus.hidden = false;
		desktopAlertsStatus.classList.toggle("on", enabled);
		if (!supported) {
			desktopAlertsStatus.textContent = "not supported";
			desktopAlertsButton.hidden = true;
			return;
		}

		desktopAlertsButton.hidden = false;
		desktopAlertsButton.textContent = enabled ? "turn off" : "turn on";
		if (Notification.permission === "denied") {
			desktopAlertsStatus.textContent = "blocked by the browser";
			desktopAlertsButton.disabled = true;
			return;
		}

		desktopAlertsStatus.textContent = enabled ? "on" : "off";
		desktopAlertsButton.disabled = false;
	};

	desktopAlertsButton.addEventListener("click", async () => {
		if (desktopAlertsActive()) {
			setDesktopAlertsEnabled(false);
			updateDesktopAlerts();
			return;
		}

		const permission = await Notification.requestPermission();
		if (permission === "granted") {
			setDesktopAlertsEnabled(true);
		}
		updateDesktopAlerts();
	});

	updateDesktopAlerts();
}

// addEditedMarker adds the 'edited' note after the given element (or at the
// end), matching the order the templates use. Replaying an event must not add
// a second marker, so a marker already present stops it.
function addEditedMarker(container, after) {
	if (!container || container.querySelector("[data-edited]")) {
		return;
	}

	const dot = document.createElement("span");
	dot.textContent = "·";
	const marker = document.createElement("span");
	marker.dataset.edited = "";
	marker.textContent = "edited";

	const fragment = document.createDocumentFragment();
	fragment.append(dot, marker);
	if (after) {
		after.after(fragment);
	} else {
		container.append(fragment);
	}
}

// updateCommentCount changes both places the thread page shows the comment
// count. Other pages carry no marked counts, so this leaves them alone.
function updateCommentCount(text) {
	if (!text) {
		return;
	}
	document.querySelectorAll("[data-comment-count]").forEach((element) => {
		element.textContent = text;
	});
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

	stream.addEventListener("notification", (event) => {
		if (!desktopAlertsActive() || !document.hidden) {
			return;
		}

		const data = JSON.parse(event.data);
		// One fixed tag means several open tabs show a single alert.
		const alert = new Notification(data.text, { tag: "soliloquy-notification" });
		alert.onclick = () => {
			// Reuse the forum tab instead of opening another one.
			window.focus();
			window.location.href = data.url;
			alert.close();
		};
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
		// A replayed event for a comment the page already has still carries the
		// current count, so apply it before the duplicate check.
		updateCommentCount(data.comment_count_text);
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
			// A replayed comment may already be deleted and has no new marker.
			const body = Array.from(inserted.children).find((child) => child.classList.contains("cb"));
			if (!body || !body.classList.contains("gone")) {
				inserted.classList.add("new");
				const header = Array.from(inserted.children).find((child) => child.classList.contains("ch"));
				const toggle = header && header.querySelector(".tog");
				if (header) {
					const tag = document.createElement("span");
					tag.className = "tag";
					tag.textContent = "new";
					header.insertBefore(tag, toggle);
				}
			}
			enhanceComments(inserted);
		}

		const empty = document.querySelector(".no-comments");
		if (empty) {
			empty.remove();
		}
	});

	stream.addEventListener("comment_edit", (event) => {
		const data = JSON.parse(event.data);
		const comment = document.getElementById("c" + data.comment_id);
		if (!comment) {
			return;
		}

		const body = Array.from(comment.children).find((child) => child.classList.contains("cb"));
		const header = Array.from(comment.children).find((child) => child.classList.contains("ch"));
		// An edit event can arrive after the delete event of the same comment,
		// because each request publishes when it finishes. A deleted comment
		// must not get its text back.
		if (!body || body.classList.contains("gone")) {
			return;
		}

		// Only the body changes. The vote, edit and delete controls differ for
		// each viewer, so the stream never sends them.
		body.innerHTML = data.html;
		if (header) {
			addEditedMarker(header, header.querySelector(".score"));
		}
	});

	stream.addEventListener("comment_delete", (event) => {
		const data = JSON.parse(event.data);
		const comment = document.getElementById("c" + data.comment_id);
		if (!comment) {
			return;
		}

		const source = document.createElement("template");
		source.innerHTML = data.html.trim();
		const deleted = source.content.firstElementChild;
		if (!deleted) {
			return;
		}

		const newHeader = Array.from(deleted.children).find((child) => child.classList.contains("ch"));
		const newBody = Array.from(deleted.children).find((child) => child.classList.contains("cb"));
		if (!newHeader || !newBody) {
			return;
		}

		Array.from(comment.children).forEach((child) => {
			if (child.classList.contains("ch")) {
				child.replaceWith(newHeader);
			} else if (child.classList.contains("cb")) {
				child.replaceWith(newBody);
			} else if (child.classList.contains("ca") || child.classList.contains("reply-form")) {
				child.remove();
			}
		});

		// Replace only the comment itself: its replies and the .collapsed class
		// on its <li> stay. The new header's toggle must still match the state.
		const toggle = newHeader.querySelector(".tog");
		if (toggle && comment.classList.contains("collapsed")) {
			toggle.textContent = "[+]";
			toggle.setAttribute("aria-expanded", "false");
			toggle.setAttribute("aria-label", "Expand comment");
		}
		// The server never marks a deleted comment as new, so drop the class
		// that no longer matches the markup.
		comment.classList.remove("new");
		enhanceComments(comment);
		updateCommentCount(data.comment_count_text);
	});

	stream.addEventListener("thread_edit", (event) => {
		const data = JSON.parse(event.data);
		const thread = document.querySelector("article.thread");
		if (!thread) {
			return;
		}

		const title = thread.querySelector(".thread-title");
		if (title) {
			title.innerHTML = data.title_html;
		}

		const main = thread.querySelector(".thread-main");
		if (main) {
			const body = Array.from(main.children).find((child) => child.classList.contains("thread-body"));
			if (data.body_html === "") {
				// The body block does not exist for a bodyless thread.
				if (body) {
					body.remove();
				}
			} else if (body) {
				body.innerHTML = data.body_html;
			} else {
				const created = document.createElement("div");
				created.className = "md thread-body";
				created.innerHTML = data.body_html;
				main.appendChild(created);
			}
		}

		const meta = thread.querySelector(".meta");
		if (meta) {
			addEditedMarker(meta, meta.querySelector('a[href^="#comments"]'));
		}
	});

	stream.addEventListener("vote", (event) => {
		const data = JSON.parse(event.data);
		let score = null;
		if (data.comment_id === null) {
			score = document.querySelector("article.thread .score");
		} else {
			const comment = document.getElementById("c" + data.comment_id);
			const header = comment && Array.from(comment.children).find((child) => child.classList.contains("ch"));
			score = header && header.querySelector(".score");
		}
		if (!score) {
			return;
		}

		// Only the number changes: the event carries no voter, so the viewer's
		// own highlight and button state stay as they are.
		score.textContent = score.dataset.scoreFormat === "number" ? data.score : data.score_text;
	});
}

// Keyboard shortcuts: n and p jump between unread comments and ? shows the
// shortcut list. The target comes from the current viewport, not a stored
// index, so manual scrolling changes where n and p go next.
const shortcutsDialogID = "keyboard-shortcuts";

// A focused unread comment that is at least partly in the viewport is the
// reference point, so repeated presses still move through unread comments that
// cannot reach the top of the viewport, such as those at the end of the page.
function jumpToUnread(forward) {
	const comments = Array.from(document.querySelectorAll(".c.new")).filter((comment) => {
		const parent = comment.parentElement;
		return !(parent && parent.closest(".c.collapsed"));
	});

	const active = document.activeElement;
	// Only the comment the last jump focused counts, and only while its own
	// header is on screen. The whole <li> also holds its replies, so its box can
	// stay on screen long after the user has scrolled past it.
	const current = active instanceof Element && active.matches(".c.new") ? active : null;
	const header = current && Array.from(current.children).find((child) => child.classList.contains("ch"));
	const rect = header && header.getBoundingClientRect();
	let target = null;
	if (rect && rect.bottom > 0 && rect.top < window.innerHeight) {
		const index = comments.indexOf(current);
		target = index === -1 ? null : comments[index + (forward ? 1 : -1)] || null;
	} else {
		for (const comment of comments) {
			const top = comment.getBoundingClientRect().top;
			if (forward) {
				if (top > 20) {
					target = comment;
					break;
				}
			} else if (top < 0) {
				target = comment;
			}
		}
	}

	if (!target) {
		return false;
	}

	target.tabIndex = -1;
	target.scrollIntoView({ behavior: "smooth", block: "start" });
	target.focus({ preventScroll: true });
	return true;
}

// shortcutsDialog builds the shortcut list the first time ? is pressed. The
// dialog is native, so Escape closes it and it traps focus by itself.
function shortcutsDialog() {
	let dialog = document.getElementById(shortcutsDialogID);
	if (dialog) {
		return dialog;
	}

	dialog = document.createElement("dialog");
	dialog.id = shortcutsDialogID;
	dialog.className = "shortcuts";
	dialog.setAttribute("aria-labelledby", "shortcuts-title");
	// The padding sits on the inner div, so a click inside the box never has
	// the dialog itself as its target.
	dialog.innerHTML = `<div class="shortcuts-body">
		<div class="shortcuts-head">
			<h2 id="shortcuts-title">keyboard shortcuts</h2>
			<button class="btn ghost" type="button" data-close>close</button>
		</div>
		<dl>
			<dt><kbd>n</kbd></dt><dd>next unread comment</dd>
			<dt><kbd>p</kbd></dt><dd>previous unread comment</dd>
			<dt><kbd>?</kbd></dt><dd>show this list</dd>
			<dt><kbd>Ctrl+Enter</kbd></dt><dd>submit form (Cmd+Enter on macOS)</dd>
		</dl>
	</div>`;

	dialog.querySelector("[data-close]").addEventListener("click", () => dialog.close());
	// A click on the backdrop reports the dialog itself as the target, but so
	// do its border and rounded corners, so check the coordinates as well.
	dialog.addEventListener("click", (event) => {
		if (event.target !== dialog) {
			return;
		}
		const box = dialog.getBoundingClientRect();
		if (
			event.clientX < box.left ||
			event.clientX > box.right ||
			event.clientY < box.top ||
			event.clientY > box.bottom
		) {
			dialog.close();
		}
	});

	document.body.append(dialog);
	return dialog;
}

document.addEventListener("keydown", (event) => {
	if (event.defaultPrevented || event.isComposing || event.repeat ||
		event.ctrlKey || event.metaKey || event.altKey) {
		return;
	}
	const field = event.target;
	if (field instanceof Element &&
		(field.isContentEditable || field.closest("input, textarea, select"))) {
		return;
	}

	if (event.key === "?") {
		event.preventDefault();
		const dialog = shortcutsDialog();
		if (dialog.open) {
			dialog.close();
		} else {
			dialog.showModal();
		}
		return;
	}

	// The shortcut list is modal; leave the page behind it alone.
	const dialog = document.getElementById(shortcutsDialogID);
	if ((event.key === "n" || event.key === "p") && !(dialog && dialog.open) &&
		jumpToUnread(event.key === "n")) {
		event.preventDefault();
	}
});
