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
		const comment = form.closest(".c");
		if (comment) {
			// A comment vote also carries the reaction counts, because the
			// arrow can remove the viewer's reaction.
			applyCommentVote(comment, data);
			return;
		}

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

// React to a comment without a page reload. The chips are plain forms, and the
// react control is a details element, so both work without JavaScript. As with
// votes, a failed request reloads the page because the reaction may already
// have been saved.
document.addEventListener("submit", async (event) => {
	const form = event.target;
	if (!(form instanceof HTMLFormElement) || !form.classList.contains("reaction-form")) {
		return;
	}

	event.preventDefault();

	const comment = form.closest(".c");
	const details = form.closest("details");
	try {
		const response = await fetch(form.action, {
			method: "POST",
			headers: { Accept: "application/json" },
			credentials: "same-origin",
			body: new URLSearchParams(new FormData(form)),
		});
		if (!response.ok) {
			window.location.reload();
			return;
		}

		const data = await response.json();
		if (comment) {
			applyCommentVote(comment, data);
		}
		if (details) {
			details.open = false;
		}
	} catch (error) {
		window.location.reload();
	}
});

// Close an open react menu when the viewer taps or clicks outside it, or
// presses Escape. pointerdown is used and not click, because iOS Safari does
// not send click events for taps on plain page content. A tap on the menu's
// own "react" label is inside the details element, so the browser closes it.
function closeReactMenus(except) {
	document.querySelectorAll("details.react[open]").forEach((details) => {
		if (except instanceof Node && details.contains(except)) {
			return;
		}
		// Keep keyboard focus on the page when it was inside the menu.
		const summary = details.contains(document.activeElement) && details.querySelector("summary");
		details.open = false;
		if (summary) {
			summary.focus();
		}
	});
}

document.addEventListener("pointerdown", (event) => {
	closeReactMenus(event.target);
});

document.addEventListener("keydown", (event) => {
	if (event.key === "Escape") {
		closeReactMenus(null);
	}
});

// The reaction keys, emoji and accessible names match reactionOptions in
// internal/web/comments_handlers.go. A live vote event carries counts rather
// than markup, so the page builds the chips itself.
const reactions = [
	{ key: "heart", emoji: "❤️", name: "Love" },
	{ key: "laugh", emoji: "😂", name: "Funny" },
	{ key: "idea", emoji: "💡", name: "Insightful" },
	{ key: "thanks", emoji: "🙏", name: "Thanks" },
	{ key: "party", emoji: "🎉", name: "Congratulations" },
];

// reactionCounts turns the response's ordered list into a lookup by key.
function reactionCounts(list) {
	const counts = new Map();
	(list || []).forEach((entry) => counts.set(entry.key, Number(entry.count) || 0));
	return counts;
}

// commentChild returns a comment's own child with the given class. Replies are
// nested inside the comment's <li>, so lookups on a comment must not reach into
// them.
function commentChild(comment, className) {
	return Array.from(comment.children).find((child) => child.classList.contains(className)) || null;
}

// applyCommentVote updates a comment's arrow, score and chips after a vote or
// a reaction response.
function applyCommentVote(comment, data) {
	const header = commentChild(comment, "ch");
	if (header) {
		const score = header.querySelector(".score");
		if (score) {
			score.textContent = score.dataset.scoreFormat === "number" ? data.score : data.score_text;
			score.classList.toggle("on", Boolean(data.voted));
		}

		const button = header.querySelector(".vote-form button");
		if (button) {
			button.classList.toggle("on", Boolean(data.voted));
			button.dataset.voted = data.voted ? "true" : "false";
			button.setAttribute("aria-pressed", data.voted ? "true" : "false");
			button.setAttribute("aria-label", data.voted ? "Remove upvote" : "Upvote");
		}
	}

	if (data.reactions) {
		updateReactions(comment, reactionCounts(data.reactions), data.reaction || "");
	}
}

// currentReaction is the reaction the viewer has highlighted on a comment.
// Live events do not say who voted, so this is how the viewer's own highlight
// survives an update from another member.
function currentReaction(comment) {
	const row = commentChild(comment, "rx");
	const chip = row && row.querySelector(".chip.on");
	return chip && chip.dataset.reaction ? chip.dataset.reaction : "";
}

// updateReactions rebuilds a comment's chip row from reaction counts. The row
// is only shown when a reaction has a count. Existing chips are reused and
// moved into order rather than recreated, so a chip keeps keyboard focus when
// the viewer's own click updates it.
function updateReactions(comment, counts, mine) {
	// A delete event can replace the comment while a vote event or a fetch
	// response is still on its way; a late update must not give it chips back.
	const body = commentChild(comment, "cb");
	if (body && body.classList.contains("gone")) {
		return;
	}

	let row = commentChild(comment, "rx");
	// The react control only exists for viewers who can vote, and it is what
	// decides whether a chip is a button or plain text. It belongs to this
	// comment, not to a reply.
	const actions = commentChild(comment, "ca");
	const canVote = Boolean(actions && actions.querySelector(".react"));
	const visible = reactions.filter((reaction) => (counts.get(reaction.key) || 0) > 0);

	if (!row) {
		if (!visible.length) {
			return;
		}
		row = document.createElement("div");
		row.className = "rx";
		if (body) {
			body.after(row);
		} else {
			comment.appendChild(row);
		}
	}

	if (!visible.length) {
		row.remove();
		return;
	}

	for (const reaction of visible) {
		const count = counts.get(reaction.key) || 0;
		let chip = Array.from(row.querySelectorAll(".chip")).find((element) => element.dataset.reaction === reaction.key);
		if (!chip) {
			let wrapper;
			if (canVote) {
				wrapper = document.createElement("form");
				wrapper.className = "reaction-form";
				wrapper.method = "post";
				wrapper.action = "/comments/" + comment.dataset.commentId + "/reaction";

				const input = document.createElement("input");
				input.type = "hidden";
				input.name = "reaction";
				input.value = reaction.key;

				chip = document.createElement("button");
				chip.type = "submit";
				wrapper.append(input, chip);
			} else {
				chip = document.createElement("span");
				wrapper = chip;
				chip.setAttribute("role", "img");
			}

			chip.className = "chip";
			chip.dataset.reaction = reaction.key;

			const emoji = document.createElement("span");
			emoji.className = "rx-emoji";
			emoji.setAttribute("aria-hidden", "true");
			emoji.textContent = reaction.emoji;

			const number = document.createElement("span");
			number.className = "rx-count";
			number.textContent = count;

			chip.append(emoji, number);
			row.appendChild(wrapper);
		}

		const active = reaction.key === mine;
		chip.classList.toggle("on", active);
		if (chip.tagName === "BUTTON") {
			chip.setAttribute("aria-pressed", active ? "true" : "false");
		}
		const number = chip.querySelector(".rx-count");
		if (number) {
			number.textContent = count;
		}
		const suffix = count === 1 ? " reaction" : " reactions";
		chip.setAttribute("aria-label", reaction.name + ", " + count + suffix);
	}

	// Drop chips whose count went to zero before ordering the rest.
	for (const child of Array.from(row.children)) {
		const chip = child.classList.contains("chip") ? child : child.querySelector(".chip");
		if (!chip || !visible.some((reaction) => reaction.key === chip.dataset.reaction)) {
			child.remove();
		}
	}

	// Move a wrapper only when it is out of place, so a row that is already
	// in order keeps keyboard focus on the chip the viewer just used.
	let previous = null;
	for (const reaction of visible) {
		const chip = Array.from(row.querySelectorAll(".chip")).find((element) => element.dataset.reaction === reaction.key);
		if (!chip) {
			continue;
		}
		const wrapper = chip.parentElement === row ? chip : chip.parentElement;
		const reference = previous ? previous.nextElementSibling : row.firstElementChild;
		if (wrapper !== reference) {
			row.insertBefore(wrapper, reference);
		}
		previous = wrapper;
	}
}

// Follow or unfollow a thread without a page reload. Like the vote form, the
// plain form still works when JavaScript is unavailable, and a failed request
// reloads the page because the follow may already have been saved.
document.addEventListener("submit", async (event) => {
	const form = event.target;
	if (!(form instanceof HTMLFormElement) || !form.classList.contains("follow-form")) {
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
		const button = form.querySelector("button");
		if (button) {
			button.textContent = data.button_text;
			button.setAttribute("aria-pressed", data.following ? "true" : "false");
		}
		form.setAttribute("action", data.action);
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

// buildReplyForm creates the inline reply form inside a comment and returns
// it, or null when the comment has nowhere to put it. The click handler below
// and the page-load draft restore both use it, so they build the same markup.
// Only the click handler focuses the textarea.
function buildReplyForm(link, comment) {
	const actions = commentChild(comment, "ca");
	if (!actions) {
		return null;
	}

	const header = commentChild(comment, "ch");
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
	note.dataset.replyNote = "";
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
		if (!confirmDiscard(form)) {
			return;
		}
		removeDraft(form);
		clearUnsaved(form);
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
	restoreDraft(form);
	updateCounter(textarea);
	return form;
}

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

	const existing = commentChild(comment, "reply-form");
	if (existing) {
		event.preventDefault();
		const textarea = existing.querySelector("textarea");
		if (textarea) {
			textarea.focus();
		}
		return;
	}

	if (!commentChild(comment, "ca")) {
		return;
	}

	event.preventDefault();

	const form = buildReplyForm(link, comment);
	const textarea = form && form.querySelector("textarea");
	if (textarea) {
		textarea.focus();
	}
});

// Quote selected text from a comment or the thread body into the reply box.
// Selecting alone inserts nothing: a floating button appears just below the
// selection and only a click on it inserts a quote. One button serves the
// whole page, and it stays hidden when the selection is empty, spans more
// than one body, or has no reply box to go into.
const quoteButton = document.createElement("button");
quoteButton.type = "button";
quoteButton.className = "btn quote-btn";
quoteButton.textContent = "quote";
quoteButton.hidden = true;
document.body.append(quoteButton);

// The quote saved when the button appeared. Touch browsers can clear the
// selection on tap, so the click cannot read it again.
let quoteState = null;
// A mouse drag selection must not move the button under the pointer.
let quoteSelecting = false;
// A press on the button briefly ignores selection changes, which would
// otherwise hide it before the click arrives.
let quotePressedUntil = 0;

function quotePaused() {
	return quoteSelecting || Date.now() < quotePressedUntil;
}

// quoteSourceAt finds the comment body or thread body that holds a node.
function quoteSourceAt(node) {
	const element = node && (node.nodeType === Node.TEXT_NODE ? node.parentElement : node);
	return element instanceof Element ? element.closest(".thread-body, .c > .cb") : null;
}

// quoteTargetFor finds the textarea that receives a quote from a source. A
// comment quotes into its own reply form, which is a direct child of its
// li.c, so several open reply boxes stay independent.
function quoteTargetFor(source) {
	let form = null;
	if (source.classList.contains("thread-body")) {
		form = document.getElementById("comment-form");
	} else {
		const comment = source.closest("li.c");
		form = comment && Array.from(comment.children).find((child) => child.classList.contains("reply-form"));
	}
	const field = form && form.querySelector('textarea[name="body"]');
	return field || null;
}

// quoteMarkdown converts a selected range inside a comment or thread body to
// Markdown. The HTML comes from internal/markdown, so the element set is
// small. A table keeps only its text, and unknown elements keep their
// children. No Markdown characters are escaped, which keeps the code small.
function quoteMarkdown(range, source) {
	const fragment = range.cloneContents();
	quotePrune(fragment);
	const root = quoteRestore(range, source, fragment);
	if (root instanceof Element) {
		return quoteConvert(root);
	}
	return quoteParts(root).map((part) => part.text).join("\n\n");
}

// quotePrune removes empty elements from the cloned fragment. cloneContents
// adds a shallow clone of an element that the range only touches, so a
// triple-click leaves empty headings, list items, links and code blocks that
// would otherwise become stray markers. An element that holds only an image,
// checkbox, rule or line break still means something, so keep it.
function quotePrune(fragment) {
	for (const element of Array.from(fragment.querySelectorAll("*")).reverse()) {
		const empty = element.textContent.trim() === "";
		const meaningful = element.matches("img, input, hr, br") || element.querySelector("img, input, hr, br");
		if (empty && !meaningful) {
			element.remove();
		}
	}
}

// quoteRestore wraps the cloned fragment in the formatting elements that
// contain the range. cloneContents leaves out the common ancestor, so text
// selected fully inside one strong, em, del, code or a would lose its
// formatting. A list comes back only when the range spans its items, because
// a selection inside one item must stay inline text.
function quoteRestore(range, source, fragment) {
	let node = range.commonAncestorContainer;
	if (node.nodeType === Node.TEXT_NODE) {
		node = node.parentElement;
	}

	const common = node;
	const wrappers = [];
	while (node && node !== source) {
		const tag = node.tagName;
		if (tag === "PRE" || tag === "CODE" || tag === "STRONG" || tag === "EM" || tag === "DEL" || tag === "A") {
			wrappers.push(node);
		} else if (node === common && (tag === "UL" || tag === "OL")) {
			wrappers.push(node);
		}
		node = node.parentElement;
	}

	let root = fragment;
	// wrappers runs from the innermost ancestor outwards, so each clone wraps
	// the previous result.
	for (const ancestor of wrappers) {
		const wrapper = ancestor.cloneNode(false);
		wrapper.append(root);
		root = wrapper;
	}
	return root;
}

const quoteBlockTags = new Set(["P", "H1", "H2", "H3", "H4", "H5", "H6", "PRE", "BLOCKQUOTE", "UL", "OL", "TABLE", "HR", "DIV"]);

function quoteBlockElement(node) {
	return node instanceof Element && quoteBlockTags.has(node.tagName);
}

// quoteParts splits a container into blocks, keeping the inline text between
// them. A list is marked so an item can put a nested list on the next line
// without a blank line, which would make the list loose.
function quoteParts(parent) {
	const parts = [];
	let inline = "";
	const flush = () => {
		const text = quoteClean(inline).trim();
		if (text !== "") {
			parts.push({ text, list: false });
		}
		inline = "";
	};

	for (const child of parent.childNodes) {
		if (quoteBlockElement(child)) {
			flush();
			const text = quoteConvert(child).trim();
			if (text !== "") {
				parts.push({ text, list: child.tagName === "UL" || child.tagName === "OL" });
			}
		} else {
			inline += quoteConvert(child);
		}
	}
	flush();
	return parts;
}

// quoteClean removes spaces around a line break. It must not collapse runs of
// spaces, because the text can hold a code span whose spaces are significant.
function quoteClean(text) {
	return text.replace(/ *\n */g, "\n");
}

// quoteConvert returns the Markdown for one node.
function quoteConvert(node) {
	if (node.nodeType === Node.TEXT_NODE) {
		return node.data.replace(/\s+/g, " ");
	}
	if (!(node instanceof Element)) {
		return "";
	}

	const tag = node.tagName;
	switch (tag) {
		case "STRONG":
			return quoteEmphasis(node, "**");
		case "EM":
			return quoteEmphasis(node, "*");
		case "DEL":
			return quoteEmphasis(node, "~~");
		case "CODE":
			return quoteCodeSpan(node);
		case "A":
			return quoteLink(node);
		case "IMG":
			return "![" + (node.getAttribute("alt") || "") + "](" + (node.getAttribute("src") || "") + ")";
		case "BR":
			return "\n";
		case "INPUT":
			return node.type === "checkbox" ? (node.checked ? "[x] " : "[ ] ") : "";
		case "HR":
			return "---";
		case "PRE":
			return quoteCodeBlock(node);
		case "BLOCKQUOTE":
			return quoteBlockquote(quoteParts(node).map((part) => part.text).join("\n\n"));
		case "UL":
		case "OL":
			return quoteList(node, tag === "OL");
		case "TABLE":
			return quoteTable(node);
		case "P":
			return quoteInline(node).trim();
	}

	if (/^H[1-6]$/.test(tag)) {
		return "#".repeat(Number(tag[1])) + " " + quoteInline(node).trim();
	}
	if (quoteBlockElement(node)) {
		return quoteParts(node).map((part) => part.text).join("\n\n");
	}
	return quoteInline(node);
}

function quoteInline(node) {
	let text = "";
	for (const child of node.childNodes) {
		text += quoteConvert(child);
	}
	return quoteClean(text);
}

// quoteEmphasis wraps inline text in a marker. Whitespace at the edges goes
// outside the markers, because CommonMark does not read **bold ** as bold.
function quoteEmphasis(node, marker) {
	const text = quoteInline(node);
	const inner = text.trim();
	if (inner === "") {
		return text;
	}
	const start = text.indexOf(inner);
	return text.slice(0, start) + marker + inner + marker + text.slice(start + inner.length);
}

// quoteCodeSpan keeps the content verbatim and uses a backtick run longer
// than any inside it, as CommonMark requires.
function quoteCodeSpan(node) {
	const text = node.textContent;
	let longest = 0;
	for (const run of text.match(/`+/g) || []) {
		longest = Math.max(longest, run.length);
	}
	const ticks = "`".repeat(longest + 1);
	const pad = text.startsWith("`") || text.endsWith("`") ? " " : "";
	return ticks + pad + text + pad + ticks;
}

function quoteLink(node) {
	const text = quoteInline(node).trim();
	const href = node.getAttribute("href") || "";
	if (text === "") {
		return "";
	}
	if (text === href) {
		return href;
	}
	return "[" + text + "](" + href + ")";
}

// quoteCodeBlock turns the pre element into a fenced block. The sanitizer
// drops the language class today, but read it if it is there.
function quoteCodeBlock(node) {
	const code = node.querySelector("code");
	const content = (code || node).textContent.replace(/\n$/, "");
	const language = code && code.className.match(/language-(\S+)/);
	let fence = 3;
	for (const run of content.match(/`+/g) || []) {
		fence = Math.max(fence, run.length + 1);
	}
	const ticks = "`".repeat(fence);
	return ticks + (language ? language[1] : "") + "\n" + content + "\n" + ticks;
}

function quoteBlockquote(text) {
	return text
		.split("\n")
		.map((line) => (line === "" ? ">" : "> " + line))
		.join("\n");
}

function quoteList(node, ordered) {
	const start = ordered ? Number(node.getAttribute("start")) || 1 : 0;
	let index = 0;
	return Array.from(node.children)
		.filter((child) => child.tagName === "LI")
		.map((item) => {
			const marker = ordered ? start + index++ + ". " : "- ";
			return quoteListItem(item, marker);
		})
		.join("\n");
}

// quoteListItem puts the first block after the marker and indents continuation
// lines under it. A nested list follows on the next line, as a tight list
// needs, while other blocks get a blank line.
function quoteListItem(item, marker) {
	const parts = quoteParts(item);
	if (parts.length === 0) {
		return marker.trimEnd();
	}
	const indent = " ".repeat(marker.length);
	let out = marker + quoteIndent(parts[0].text, indent, true);
	for (let i = 1; i < parts.length; i++) {
		out += (parts[i].list ? "\n" : "\n\n") + quoteIndent(parts[i].text, indent, false);
	}
	return out;
}

function quoteIndent(text, indent, skipFirst) {
	return text
		.split("\n")
		.map((line, index) => (index === 0 && skipFirst) || line === "" ? line : indent + line)
		.join("\n");
}

function quoteTable(node) {
	return Array.from(node.querySelectorAll("tr"))
		.map((row) => {
			const cells = Array.from(row.children).filter((cell) => cell.tagName === "TH" || cell.tagName === "TD");
			return cells
				.map((cell) => quoteParts(cell).map((part) => part.text).join(" ").replace(/\s*\n\s*/g, " "))
				.join(" | ");
		})
		.join("\n");
}

// quoteBlock prefixes each line so the quote stays one Markdown block. Empty
// lines become ">", and no leading or trailing empty lines remain.
function quoteBlock(text) {
	const lines = text.replace(/\r\n?/g, "\n").split("\n");
	while (lines.length > 0 && lines[0].trim() === "") {
		lines.shift();
	}
	while (lines.length > 0 && lines[lines.length - 1].trim() === "") {
		lines.pop();
	}
	return lines.map((line) => (line.trim() === "" ? ">" : "> " + line)).join("\n");
}

// quoteSelection returns the quote and where to put it, or null when the
// selection cannot be quoted. Chrome ends a triple-click at the start of the
// next block, so accept an end just past the source when only whitespace
// follows, and clip the range to the source.
function quoteSelection(selection) {
	if (!selection || selection.rangeCount === 0 || selection.isCollapsed) {
		return null;
	}

	const range = selection.getRangeAt(0);
	const source = quoteSourceAt(range.startContainer);
	if (!source) {
		return null;
	}
	const target = quoteTargetFor(source);
	if (!target) {
		return null;
	}

	const clipped = range.cloneRange();
	if (!source.contains(range.endContainer)) {
		const sourceEnd = document.createRange();
		sourceEnd.selectNodeContents(source);
		const tail = document.createRange();
		tail.setStart(sourceEnd.endContainer, sourceEnd.endOffset);
		tail.setEnd(range.endContainer, range.endOffset);
		if (tail.toString().trim() !== "") {
			return null;
		}
		clipped.setEnd(sourceEnd.endContainer, sourceEnd.endOffset);
	}

	const text = quoteMarkdown(clipped, source);
	if (text.trim() === "") {
		return null;
	}

	const rects = clipped.getClientRects();
	const rect = rects.length > 0 ? rects[rects.length - 1] : clipped.getBoundingClientRect();
	return { text: quoteBlock(text), target, rect };
}

// positionQuoteButton puts the button just below the last line of the
// selection, in page coordinates so it scrolls with the text. The horizontal
// position stays inside the viewport, and a coarse pointer leaves room for
// the native selection handles.
function positionQuoteButton(rect) {
	quoteButton.hidden = false;
	const width = quoteButton.offsetWidth;
	const margin = 8;
	const room = window.matchMedia("(pointer: coarse)").matches ? 24 : 0;
	const limit = document.documentElement.clientWidth - width - margin;
	const left = Math.max(margin, Math.min(rect.right, limit));
	const top = rect.bottom + room + 4;
	quoteButton.style.left = left + window.scrollX + "px";
	quoteButton.style.top = top + window.scrollY + "px";
}

function hideQuoteButton() {
	quoteButton.hidden = true;
	quoteState = null;
}

function updateQuoteButton() {
	if (quotePaused()) {
		return;
	}
	const found = quoteSelection(window.getSelection());
	if (!found) {
		hideQuoteButton();
		return;
	}
	quoteState = { text: found.text, target: found.target };
	positionQuoteButton(found.rect);
}

// insertQuote puts the Markdown quote at the caret of the target textarea,
// replacing any selected text there. It keeps a blank line around the quote
// unless one is already there, and leaves the caret on an empty line between
// the quote and any text that follows, so typing does not extend the quote.
// The write tab is shown first, so the quote is visible.
function insertQuote(target, markdown) {
	const start = target.selectionStart;
	const end = target.selectionEnd;
	const before = target.value.slice(0, start);
	const after = target.value.slice(end);

	let prefix = "";
	if (before !== "" && !before.endsWith("\n\n")) {
		prefix = before.endsWith("\n") ? "\n" : "\n\n";
	}
	let suffix = "";
	if (after !== "" && !after.startsWith("\n\n")) {
		suffix = after.startsWith("\n") ? "\n" : "\n\n";
	}

	// The newlines after the quote always put the caret on its own empty line.
	// The suffix adds the blank line before following text unless the text
	// already starts with one.
	const insertion = prefix + markdown + "\n\n" + suffix;
	const caret = start + prefix.length + markdown.length + 2;

	const form = target.closest("form");
	const writeTab = form && form.querySelector('[data-tab="write"]');
	if (target.hidden && writeTab) {
		writeTab.click();
	}

	// insertText keeps the native undo history and fires the input event that
	// updates the counter and the preview. setRangeText is the fallback for
	// browsers where insertText is unavailable or disabled.
	target.focus();
	target.setSelectionRange(start, end);
	let inserted = false;
	try {
		inserted = document.execCommand("insertText", false, insertion);
	} catch (error) {
		inserted = false;
	}
	if (!inserted) {
		target.setRangeText(insertion, start, end, "end");
		target.dispatchEvent(new Event("input", { bubbles: true }));
	}
	target.setSelectionRange(caret, caret);
}

quoteButton.addEventListener("mousedown", (event) => {
	// Keep the selection (and the caret in a focused textarea) while the
	// button is pressed; the click uses the state saved when it appeared.
	event.preventDefault();
	quotePressedUntil = Date.now() + 1000;
});

quoteButton.addEventListener("pointerdown", () => {
	quotePressedUntil = Date.now() + 1000;
});

quoteButton.addEventListener("click", () => {
	const state = quoteState;
	hideQuoteButton();
	quotePressedUntil = 0;
	const selection = window.getSelection();
	if (selection) {
		selection.removeAllRanges();
	}
	if (state && state.target.isConnected) {
		insertQuote(state.target, state.text);
	}
});

document.addEventListener("selectionchange", updateQuoteButton);

// A mouse drag runs from mousedown to mouseup, so the button appears only
// once the drag ends and never jumps under the pointer. Touch selection
// handles only send selectionchange, which updates the button directly.
document.addEventListener("mousedown", (event) => {
	if (event.button === 0) {
		quoteSelecting = true;
		if (!quoteButton.contains(event.target)) {
			hideQuoteButton();
		}
	}
});

document.addEventListener("mouseup", (event) => {
	if (event.button === 0) {
		quoteSelecting = false;
		updateQuoteButton();
	}
});

// A click can remove the reply box the button points at, or clear the
// selection, without a selectionchange.
document.addEventListener("click", updateQuoteButton);

window.addEventListener("resize", updateQuoteButton);

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
					// The fill sets the value directly, without an input event,
					// so save the draft here or the title is lost on a reload.
					saveDraft(threadForm);
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

// Drafts. Text typed into the thread form, the comment form and the reply and
// edit forms is kept in localStorage, so a reload, a closed tab or a failed
// post does not lose it. A draft belongs to the member and to the address its
// form posts to: the inline reply form and the standalone reply page share a
// draft, and another account on the same browser sees nothing. Pages without
// data-user, meaning visitors, keep no drafts.
const draftUser = document.body.dataset.user || "";
const draftFields = ["body", "title", "url"];

// Storage can be full, blocked or absent. Every access is wrapped, and a form
// whose save failed is marked so leaving the page can still warn.
function draftStorageKey(path) {
	return "draft:" + draftUser + ":" + path;
}

function draftKey(form) {
	return draftStorageKey(new URL(form.action, window.location.href).pathname);
}

// draftForm is true for the forms this feature covers: the member's forms that
// hold a body textarea. Vote, follow, search and settings forms keep no draft.
function draftForm(form) {
	return draftUser !== "" && form.querySelector('textarea[name="body"]') !== null;
}

function storedDraft(key) {
	try {
		return window.localStorage.getItem(key);
	} catch (error) {
		return null;
	}
}

function hasDraft(form) {
	return draftForm(form) && storedDraft(draftKey(form)) !== null;
}

function removeDraft(form) {
	if (!draftForm(form)) {
		return;
	}
	try {
		window.localStorage.removeItem(draftKey(form));
	} catch (error) {
		// Storage is unavailable; there is nothing else to remove.
	}
}

// draftValues collects the fields a form's draft keeps. Only the thread form
// has a title and a url; the other forms just carry the body.
function draftValues(form) {
	const values = {};
	draftFields.forEach((name) => {
		const field = form.querySelector('[name="' + name + '"]');
		if (field) {
			values[name] = field.value;
		}
	});
	return values;
}

// saveDraft writes the form's text on every change and reports whether the
// text is now protected. An emptied form drops its draft instead. If the write
// fails the text is unprotected: the form is marked and leaving the page asks
// first.
function saveDraft(form) {
	if (!draftForm(form)) {
		return true;
	}

	const values = draftValues(form);
	const empty = Object.values(values).every((value) => value.trim() === "");
	if (empty) {
		removeDraft(form);
		clearUnsaved(form);
		return true;
	}

	try {
		window.localStorage.setItem(draftKey(form), JSON.stringify(values));
	} catch (error) {
		markUnsaved(form);
		return false;
	}
	clearUnsaved(form);
	return true;
}

// restoreDraft replaces the server's values with the saved ones, which is what
// the member last typed. On the error re-render the two are equal, so nothing
// changes and no note appears. It returns whether anything changed.
function restoreDraft(form) {
	if (!draftForm(form)) {
		return false;
	}

	const stored = storedDraft(draftKey(form));
	if (stored === null) {
		return false;
	}

	let values;
	try {
		values = JSON.parse(stored);
	} catch (error) {
		return false;
	}
	if (!values || typeof values !== "object") {
		return false;
	}

	let changed = false;
	draftFields.forEach((name) => {
		if (typeof values[name] !== "string") {
			return;
		}
		const field = form.querySelector('[name="' + name + '"]');
		if (field && field.value !== values[name]) {
			field.value = values[name];
			changed = true;
		}
	});
	if (!changed) {
		return false;
	}

	form.querySelectorAll("textarea, input[data-counter-name]").forEach(updateCounter);
	showDraftNote(form);
	return true;
}

// showDraftNote adds the 'draft restored' line once, next to the form's
// counter or actions. It is text only; the design has no such note, so it is
// kept to the same muted style as the help text around it.
function showDraftNote(form) {
	if (form.querySelector("[data-draft-note]")) {
		return;
	}

	const note = document.createElement("span");
	note.className = "draft-note";
	note.dataset.draftNote = "";
	note.textContent = "draft restored";

	const bar = form.querySelector(".edbar");
	if (bar) {
		const counter = bar.querySelector("[data-counter]");
		if (counter) {
			counter.after(note);
		} else {
			bar.append(note);
		}
		return;
	}

	const actions = form.querySelector(".thread-form-actions");
	if (actions) {
		actions.append(note);
	}
}

// The beforeunload listener exists only while some form's text could not be
// saved. A permanent listener would keep the browser's back/forward cache from
// working, so the marker on the form decides when to remove it.
function warnBeforeLeaving(event) {
	if (!document.querySelector("[data-draft-unsaved]")) {
		window.removeEventListener("beforeunload", warnBeforeLeaving);
		return;
	}
	event.preventDefault();
	// Chrome shows its own message only when returnValue is set.
	event.returnValue = "";
}

function markUnsaved(form) {
	form.dataset.draftUnsaved = "";
	window.addEventListener("beforeunload", warnBeforeLeaving);
}

function clearUnsaved(form) {
	delete form.dataset.draftUnsaved;
	if (!document.querySelector("[data-draft-unsaved]")) {
		window.removeEventListener("beforeunload", warnBeforeLeaving);
	}
}

// confirmDiscard asks before a cancel throws text away. A draft in storage or a
// form whose save failed both count as text that would be lost.
function confirmDiscard(form) {
	if (!hasDraft(form) && !form.hasAttribute("data-draft-unsaved")) {
		return true;
	}
	return window.confirm("Discard this draft?");
}

// cookieValue returns a cookie's decoded value, or null. Go may quote the
// value, so the quotes come off before decoding.
function cookieValue(name) {
	const entry = document.cookie.split(";")
		.map((part) => part.trim())
		.find((part) => part.startsWith(name + "="));
	if (!entry) {
		return null;
	}

	let value = entry.slice(name.length + 1);
	if (value.startsWith('"') && value.endsWith('"')) {
		value = value.slice(1, -1);
	}
	try {
		return decodeURIComponent(value);
	} catch (error) {
		return value;
	}
}

// clearPostedDraft removes the draft of a form that was posted successfully
// and expires the marker cookie.
function clearPostedDraft() {
	const posted = cookieValue("draft_posted");
	if (posted === null) {
		return;
	}

	document.cookie = "draft_posted=; Path=/; Max-Age=0; SameSite=Lax" +
		(window.location.protocol === "https:" ? "; Secure" : "");

	if (draftUser === "" || posted === "") {
		return;
	}
	try {
		window.localStorage.removeItem(draftStorageKey(posted));
	} catch (error) {
		// Storage is unavailable, so there is no draft to remove.
	}
}

document.addEventListener("input", (event) => {
	if (event.target.form instanceof HTMLFormElement) {
		saveDraft(event.target.form);
	}
});

// Save once more on submit. The post itself must never ask about unsaved text,
// so the warning marker goes even when the final save failed. A failed save here
// leaves an older draft behind, while a validation error re-renders the newer
// submitted text, so drop the older draft rather than let it replace that copy.
// A submit that another handler cancelled posts nothing, so it changes nothing
// here: the reply box kept on a deleted comment must keep its draft and warning.
document.addEventListener("submit", (event) => {
	const form = event.target;
	if (event.defaultPrevented || !(form instanceof HTMLFormElement) || !draftForm(form)) {
		return;
	}
	if (!saveDraft(form)) {
		removeDraft(form);
	}
	clearUnsaved(form);
});

// The cancel links on the reply, edit and thread forms drop their draft.
document.addEventListener("click", (event) => {
	const link = event.target.closest("a[data-draft-cancel]");
	if (!link || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
		return;
	}

	const form = link.closest("form");
	if (!form || !draftForm(form)) {
		return;
	}
	if (!confirmDiscard(form)) {
		event.preventDefault();
		return;
	}
	removeDraft(form);
	clearUnsaved(form);
});

// A successful post leaves this cookie on the redirect. Clear its draft before
// any form restores it, so the page shows the server's result. The cookie is
// expired even for a visitor, who has no draft to remove.
clearPostedDraft();

if (draftUser !== "") {
	document.querySelectorAll("form").forEach((form) => {
		restoreDraft(form);
	});

	// A reply draft opens its box where the comment is. This must not focus the
	// textarea or scroll to it, because the member was reading the page.
	document.querySelectorAll(".c").forEach((comment) => {
		const actions = commentChild(comment, "ca");
		const link = actions && actions.querySelector("a.reply-link");
		if (!link) {
			return;
		}
		const path = new URL(link.href, window.location.href).pathname;
		if (storedDraft(draftStorageKey(path)) !== null) {
			buildReplyForm(link, comment);
		}
	});
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

// preventReplySubmit stops a reply box kept on a deleted comment from posting.
function preventReplySubmit(event) {
	event.preventDefault();
}

// keepDeletedReplyForm leaves a non-empty reply box on a deleted comment so its
// text can still be copied, but the box must not post. Disabling the button is
// not enough, because Ctrl+Enter calls form.requestSubmit() without it.
function keepDeletedReplyForm(form) {
	const note = form.querySelector("[data-reply-note]");
	if (note) {
		note.textContent = "comment deleted · copy your text";
	}

	const submit = form.querySelector('button[type="submit"]');
	if (submit) {
		submit.disabled = true;
	}

	form.addEventListener("submit", preventReplySubmit);
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
			} else if (child.classList.contains("reply-form")) {
				// A deleted comment has no reply link, so a draft in a box left
				// here could never be restored. Keep text the member can copy.
				const textarea = child.querySelector('textarea[name="body"]');
				if (textarea && textarea.value.trim() !== "") {
					keepDeletedReplyForm(child);
				} else {
					child.remove();
				}
			} else if (child.classList.contains("ca") || child.classList.contains("rx")) {
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
		if (data.comment_id === null) {
			const score = document.querySelector("article.thread .score");
			if (score) {
				score.textContent = score.dataset.scoreFormat === "number" ? data.score : data.score_text;
			}
			return;
		}

		const comment = document.getElementById("c" + data.comment_id);
		if (!comment) {
			return;
		}

		const header = Array.from(comment.children).find((child) => child.classList.contains("ch"));
		const score = header && header.querySelector(".score");
		if (score) {
			// Only the number changes: the event carries no voter, so the
			// viewer's own highlight and button state stay as they are.
			score.textContent = score.dataset.scoreFormat === "number" ? data.score : data.score_text;
		}

		// Reaction counts are the same for every viewer, but the highlight is
		// the viewer's own and is kept from the page.
		if (data.reactions) {
			updateReactions(comment, reactionCounts(data.reactions), currentReaction(comment));
		}
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
