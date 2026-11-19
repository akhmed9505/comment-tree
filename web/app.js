"use strict";

const list = document.querySelector("#comment-list");
const rootForm = document.querySelector("#root-form");
const dialog = document.querySelector("#delete-dialog");
const refresh = document.querySelector("#refresh");
const notice = document.querySelector("#notice");
const loadError = document.querySelector("#load-error");
let comments = [];
let loading = false;
let mutating = false;
let replyForm = null;
let deleteTarget = null;

const messages = {
  invalid_input: "Проверьте имя и текст комментария.",
  comment_not_found: "Комментарий уже удалён. Обновите обсуждения.",
  parent_not_found: "Комментарий, на который вы отвечаете, уже удалён.",
  result_too_large: "Обсуждение слишком большое для загрузки. Попробуйте позже.",
  request_too_large: "Комментарий слишком большой. Сократите текст.",
  concurrent_conflict: "Обсуждение изменилось. Обновите страницу и повторите действие.",
  lock_unavailable: "Обсуждение сейчас изменяется. Попробуйте ещё раз.",
  unavailable: "Сервис временно недоступен. Попробуйте позже.",
  deadline_exceeded: "Сервер не успел ответить. Обновите обсуждения перед повторной отправкой.",
  query_interrupted: "Запрос прерван. Попробуйте ещё раз.",
};

async function api(path, options = {}) {
  let response;
  try {
    response = await fetch(path, options);
  } catch {
    throw new Error("Не удалось связаться с сервером. Проверьте соединение и обновите обсуждения.");
  }
  if (response.status === 204) return null;
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error("Не удалось прочитать ответ сервера. Попробуйте позже.");
  }
  if (!response.ok) throw new Error(messages[data?.error?.code] || "Не удалось выполнить действие. Попробуйте позже.");
  return data;
}

function showMessage(element, text) {
  element.textContent = text;
  element.hidden = !text;
}

function syncControls() {
  const busy = loading || mutating;
  document.querySelectorAll("button, input, textarea").forEach((control) => { control.disabled = busy; });
  refresh.textContent = loading ? "Обновляем…" : "Обновить";
  list.setAttribute("aria-busy", String(loading));
}

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function renderComments() {
  const byID = new Map();
  for (const comment of comments) {
    if (typeof comment.author !== "string" || typeof comment.body !== "string" || typeof comment.created_at !== "string" || typeof comment.id !== "string" || (comment.parent_id !== null && typeof comment.parent_id !== "string") || byID.has(comment.id)) {
      throw new Error("Сервер вернул некорректный список комментариев.");
    }
    byID.set(comment.id, comment);
  }
  const children = new Map();
  const roots = [];
  for (const comment of comments) {
    if (comment.parent_id === null || !byID.has(comment.parent_id)) roots.push(comment);
    else {
      if (!children.has(comment.parent_id)) children.set(comment.parent_id, []);
      children.get(comment.parent_id).push(comment);
    }
  }
  const fragment = document.createDocumentFragment();
  const stack = roots.toReversed().map((comment) => ({ comment, depth: 0 }));
  const visited = new Set();
  // A flat DOM and an explicit stack avoid browser nesting and call-stack limits.
  while (stack.length) {
    const { comment, depth } = stack.pop();
    if (visited.has(comment.id)) throw new Error("Сервер вернул некорректное дерево комментариев.");
    visited.add(comment.id);
    const article = element("article", `comment depth-${Math.min(depth, 5)}`);
    article.id = `comment-${comment.id}`;
    article.dataset.commentId = comment.id;
    const meta = element("div", "comment-meta");
    const time = element("time");
    time.dateTime = comment.created_at;
    const date = new Date(comment.created_at);
    time.textContent = Number.isNaN(date.getTime()) ? "" : date.toLocaleString("ru-RU", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", year: "numeric" });
    meta.append(element("span", "author", comment.author), time);
    article.append(meta);
    if (depth > 0) {
      const parent = byID.get(comment.parent_id);
      article.append(element("p", "reply-context", `В ответ ${parent?.author || "на комментарий"}${depth > 5 ? ` · уровень ${depth}` : ""}`));
    }
    article.append(element("p", "comment-body", comment.body));
    const actions = element("div", "comment-actions");
    const reply = element("button", "", "Ответить");
    reply.type = "button";
    reply.addEventListener("click", () => openReply(article, comment));
    const remove = element("button", "delete-button", "Удалить");
    remove.type = "button";
    remove.setAttribute("aria-label", `Удалить комментарий ${comment.author}`);
    remove.addEventListener("click", () => {
      deleteTarget = comment.id;
      showMessage(document.querySelector("#delete-error"), "");
      dialog.showModal();
    });
    actions.append(reply, remove);
    article.append(actions);
    fragment.append(article);
    const replies = children.get(comment.id) || [];
    for (let i = replies.length - 1; i >= 0; i--) stack.push({ comment: replies[i], depth: depth + 1 });
  }
  if (visited.size !== comments.length) throw new Error("Сервер вернул неполное дерево комментариев.");
  list.replaceChildren(fragment);
  replyForm = null;
  document.querySelector("#comment-count").textContent = String(comments.length);
}

async function loadComments() {
  if (loading) return;
  loading = true;
  showMessage(loadError, "");
  document.querySelector("#empty").hidden = true;
  document.querySelector("#loading").hidden = comments.length > 0;
  syncControls();
  try {
    const data = await api("/comments");
    if (!data || !Array.isArray(data.comments)) throw new Error("Сервер вернул некорректный список комментариев.");
    const previous = comments;
    comments = data.comments;
    try { renderComments(); } catch (error) { comments = previous; throw error; }
    document.querySelector("#empty").hidden = comments.length !== 0;
    document.querySelector("#page-note").hidden = !data.next_cursor;
  } catch (error) {
    showMessage(loadError, error.message);
  } finally {
    loading = false;
    document.querySelector("#loading").hidden = true;
    syncControls();
  }
}

function openReply(article, comment) {
  if (replyForm?.dataset.parentId === comment.id) {
    replyForm.elements.body.focus();
    return;
  }
  if (replyForm?.elements.body.value && !window.confirm("Открыть другой ответ? Текущий черновик ответа будет потерян.")) return;
  replyForm?.remove();
  replyForm = rootForm.cloneNode(true);
  replyForm.removeAttribute("id");
  replyForm.classList.add("reply-form");
  replyForm.reset();
  replyForm.dataset.parentId = comment.id;
  replyForm.setAttribute("aria-label", `Ответ для ${comment.author}`);
  replyForm.querySelector("h2").textContent = `Ответ для ${comment.author}`;
  replyForm.elements.author.value = rootForm.elements.author.value;
  showMessage(replyForm.querySelector(".form-error"), "");
  const cancel = element("button", "", "Отмена");
  cancel.type = "button";
  cancel.addEventListener("click", () => { replyForm.remove(); replyForm = null; });
  replyForm.querySelector(".form-actions").append(cancel);
  bindForm(replyForm);
  article.append(replyForm);
  replyForm.elements.author.focus();
}

function bindForm(form) {
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (mutating || loading) return;
    const author = form.elements.author.value;
    const body = form.elements.body.value;
    const errorBox = form.querySelector(".form-error");
    if (!author.trim() || Array.from(author.trim()).length > 80 || !body.trim() || Array.from(body.trim()).length > 10000) {
      showMessage(errorBox, "Укажите имя до 80 символов и текст до 10 000 символов. Поля не могут быть пустыми.");
      return;
    }
    mutating = true;
    syncControls();
    showMessage(errorBox, "");
    showMessage(notice, "");
    try {
      await api("/comments", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ author, body, parent_id: form.dataset.parentId || null }) });
      form.elements.body.value = "";
      showMessage(notice, form.dataset.parentId ? "Ответ опубликован." : "Обсуждение опубликовано.");
      await loadComments();
    } catch (error) {
      showMessage(errorBox, error.message);
    } finally {
      mutating = false;
      syncControls();
    }
  });
}

refresh.addEventListener("click", () => {
  if (replyForm?.elements.body.value && !window.confirm("Обновить обсуждения? Черновик ответа будет потерян.")) return;
  loadComments();
});
document.querySelector("#confirm-delete").addEventListener("click", async () => {
  if (mutating || !deleteTarget) return;
  mutating = true;
  syncControls();
  try {
    await api(`/comments/${encodeURIComponent(deleteTarget)}`, { method: "DELETE" });
    dialog.close();
    showMessage(notice, "Комментарий и ответы на него удалены.");
    await loadComments();
    document.querySelector("#discussion-title").focus();
  } catch (error) {
    showMessage(document.querySelector("#delete-error"), error.message);
  } finally {
    mutating = false;
    syncControls();
  }
});
dialog.addEventListener("cancel", (event) => { if (mutating) event.preventDefault(); });
dialog.addEventListener("close", () => { deleteTarget = null; });
bindForm(rootForm);
loadComments();
