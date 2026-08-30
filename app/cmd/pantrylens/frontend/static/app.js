// PantryLens's own chat frontend. Talks to ADK's REST API (the "api"
// sublauncher, google.golang.org/adk/v2/server/adkrest) at same-origin
// /api/... -- no CORS setup needed, unlike ADK's own webui sublauncher.
// Endpoints used: POST /apps/{app}/users/{user}/sessions (create session),
// GET /apps/{app}/users/{user}/sessions/{sessionId} (fetch a session's full
// event history, to resume it on page load -- see resumeSession), POST
// /run (send a message, get back the full list of resulting events).
// Also talks to a few small PantryLens-owned endpoints (see ../frontend.go),
// none of them part of ADK's REST API: GET/PUT /profile/{userId} for
// remembering servings/cuisine preferences between visits, POST
// /detect-ingredients for photo-based ingredient detection, POST /recipes
// to save one, GET /recipes?userId=... to list "My saved recipes", and GET
// /recipes/{id} for one recipe's own standalone page (a normal same-tab
// link from that list, not popped open automatically on save).
//
// The intake form below composes a single natural-language opening message
// from structured fields (ingredients, dietary lens(es), servings, cuisine)
// and sends it through the same /run flow as any other chat turn -- there's
// no separate "structured" API, the agent just receives a well-formed first
// message (see the note about this in ../../../prompts.go). Recipes come
// back as propose_recipe tool calls (see ../../../tools_adk.go), rendered
// here as cards; the lens-preset chip values below (see index.html's
// .lens-preset-checkbox elements) must stay in sync with core/lens.go's
// built-in DietaryLens.Name values, since there's no REST endpoint to list
// them outside of a chat turn. The user can select more than one lens at
// once (e.g. Vegetarian + GERD) -- see the "Dietary lens multi-select"
// section below.

const APP_NAME = "recipe_concierge";

const chatEl = document.getElementById("chat");
const composerEl = document.getElementById("composer");
const inputEl = document.getElementById("message-input");
const sendBtn = document.getElementById("send-btn");
const newSessionBtn = document.getElementById("new-session");
const myRecipesBtn = document.getElementById("my-recipes-btn");
const myRecipesEl = document.getElementById("my-recipes");
const myRecipesBackBtn = document.getElementById("my-recipes-back");
const myRecipesTabsEl = document.getElementById("my-recipes-tabs");
const myRecipesListEl = document.getElementById("my-recipes-list");

const intakeEl = document.getElementById("intake");
const ingredientInput = document.getElementById("ingredient-input");
const ingredientChipsEl = document.getElementById("ingredient-chips");
const photoDetectBtn = document.getElementById("photo-detect-btn");
const photoDetectInput = document.getElementById("photo-detect-input");
const photoDetectStatus = document.getElementById("photo-detect-status");
const lensChipsEl = document.getElementById("lens-chips");
const lensPresetCheckboxes = Array.from(document.querySelectorAll(".lens-preset-checkbox"));
const lensNoneCheckbox = document.getElementById("lens-none-checkbox");
const lensCustomCheckbox = document.getElementById("lens-custom-checkbox");
const customLensText = document.getElementById("custom-lens-text");
const modeToggle = document.getElementById("mode-toggle");
const servingsLabel = document.getElementById("servings-label");
const servingsInput = document.getElementById("servings-input");
const mealCountField = document.getElementById("meal-count-field");
const mealCountInput = document.getElementById("meal-count-input");
const mealCountQuickPicks = document.getElementById("meal-count-quick-picks");
const mealTypeQuickPicks = document.getElementById("meal-type-quick-picks");
const cuisineInput = document.getElementById("cuisine-input");
const cuisineQuickPicks = document.getElementById("cuisine-quick-picks");
const findRecipesBtn = document.getElementById("find-recipes");
const ingredientsHintEl = document.getElementById("ingredients-hint");
const intakeErrorEl = document.getElementById("intake-error");

let userId = localStorage.getItem("pantrylens_user_id");
if (!userId) {
  userId = "user-" + crypto.randomUUID();
  localStorage.setItem("pantrylens_user_id", userId);
}
let sessionId = null;
let ingredients = [];
let mode = "tonight"; // "tonight" | "mealprep" -- see the mode-toggle handler below
let mealType = ""; // "" | "Breakfast" | "Lunch" | "Dinner" | "Snack" -- optional, user's call, not assumed
// mealPrepBatchLabel groups every recipe saved from the same meal-prep
// batch together in "My saved recipes" (see core.SavedRecipe
// .MealPrepBatch). Deliberately NOT set from the intake form's mode
// toggle -- a user can ask for meal prep by just typing it into the chat
// composer mid-conversation, skipping the toggle entirely, and the agent
// handles that fine (see prompts.go step 2b, which isn't tied to the
// structured intake message). So instead this is computed lazily, the
// first time any recipe card actually carries a storageNote -- the
// agent's own signal that it's treating this as a meal-prep item (see
// propose_recipe's storageNote field) -- via currentMealPrepBatchLabel()
// below, and reused for the rest of the session from then on.
let mealPrepBatchLabel = "";

function currentMealPrepBatchLabel() {
  if (!mealPrepBatchLabel) {
    mealPrepBatchLabel =
      "Meal prep -- " + new Date().toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  }
  return mealPrepBatchLabel;
}

const TOOL_LABELS = {
  list_lens_presets: "📋 Checking available dietary lenses",
  get_lens_preset: "📖 Reading lens details",
  save_custom_lens: "💾 Saving your custom lens",
  check_recipe_against_lens: "🔍 Checking recipe against your lens",
};

// --- Intake form -----------------------------------------------------

function renderIngredientChips() {
  ingredientChipsEl.innerHTML = "";
  ingredients.forEach((ing, i) => {
    const chip = document.createElement("div");
    chip.className = "chip";
    const label = document.createElement("span");
    label.textContent = ing;
    const remove = document.createElement("button");
    remove.type = "button";
    remove.setAttribute("aria-label", `Remove ${ing}`);
    remove.textContent = "✕";
    remove.addEventListener("click", () => {
      ingredients.splice(i, 1);
      renderIngredientChips();
    });
    chip.appendChild(label);
    chip.appendChild(remove);
    ingredientChipsEl.appendChild(chip);
  });
  findRecipesBtn.disabled = ingredients.length === 0;
  ingredientsHintEl.hidden = ingredients.length !== 0;
}

// addIngredientsFromText splits on commas AND newlines, so a single paste
// of a shopping list ("eggs, milk\nspinach, feta") or a receipt-style
// one-per-line list becomes multiple chips at once instead of one literal
// blob -- typing and hitting Enter/comma still works the same, since a
// single item just splits into a list of one. Case-insensitively dedupes
// against what's already added, since that's exactly where duplicates tend
// to sneak in (pasting an overlapping list, or the same list twice).
function addIngredientsFromText(text) {
  const seen = new Set(ingredients.map((i) => i.toLowerCase()));
  const items = [];
  for (const raw of text.split(/[,\n]+/)) {
    const item = raw.trim();
    const key = item.toLowerCase();
    if (!item || seen.has(key)) continue;
    seen.add(key);
    items.push(item);
  }
  if (items.length === 0) return;
  ingredients.push(...items);
  renderIngredientChips();
}

function addIngredientFromInput() {
  addIngredientsFromText(ingredientInput.value);
  ingredientInput.value = "";
}

ingredientInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter" || e.key === ",") {
    e.preventDefault();
    addIngredientFromInput();
  }
});
ingredientInput.addEventListener("paste", (e) => {
  const text = e.clipboardData && e.clipboardData.getData("text");
  if (text && /[,\n]/.test(text)) {
    e.preventDefault();
    addIngredientsFromText(text);
  }
  // else: no separator in the pasted text, let the default paste happen
  // (fills the input normally, same as typing).
});
ingredientInput.addEventListener("blur", () => {
  if (ingredientInput.value.trim()) addIngredientFromInput();
});

// --- Photo-based ingredient detection -----------------------------------
//
// POST /detect-ingredients (see ../frontend.go), another plain
// PantryLens-owned endpoint outside /api -- like /profile, this runs
// before any chat session exists, and is a one-shot vision classification
// call directly against Gemini (see ../../vision.go), not something that
// needs the conversational agent loop.

photoDetectBtn.addEventListener("click", () => photoDetectInput.click());

photoDetectInput.addEventListener("change", async () => {
  const file = photoDetectInput.files[0];
  if (!file) return;

  photoDetectBtn.disabled = true;
  photoDetectStatus.hidden = false;
  photoDetectStatus.className = "photo-detect-status";
  photoDetectStatus.textContent = "Looking at your photo...";

  try {
    const res = await fetch("/detect-ingredients", {
      method: "POST",
      headers: { "Content-Type": file.type || "application/octet-stream" },
      body: file,
    });
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      throw new Error(`${res.status} ${res.statusText}: ${body}`);
    }
    const data = await res.json();
    const detected = Array.isArray(data.ingredients) ? data.ingredients : [];
    if (detected.length === 0) {
      photoDetectStatus.textContent =
        "Couldn't spot any ingredients in that photo -- try another, or add them manually.";
    } else {
      addIngredientsFromText(detected.join(","));
      photoDetectStatus.textContent = `Added ${detected.length} ingredient${detected.length === 1 ? "" : "s"} from your photo.`;
    }
  } catch (err) {
    photoDetectStatus.className = "photo-detect-status error";
    photoDetectStatus.textContent = "Couldn't read that photo: " + err.message;
  } finally {
    photoDetectBtn.disabled = false;
    photoDetectInput.value = ""; // allow re-selecting the same file
  }
});

// --- Dietary lens multi-select -------------------------------------------
//
// Any number of preset chips can be active at once (e.g. Vegetarian +
// GERD), plus an optional Custom description on top of them -- "No
// restrictions" is the odd one out: it's mutually exclusive with
// everything else (selecting it clears every preset/custom selection, and
// selecting anything else clears it), and auto-reasserts itself if the
// user unchecks their way back down to nothing selected, so there's never
// an ambiguous "nothing picked" state for composeIntakeMessage to handle.

function clearNoneSelection() {
  if (lensNoneCheckbox.checked) lensNoneCheckbox.checked = false;
}

function reassertNoneIfNothingElseSelected() {
  const anyPresetChecked = lensPresetCheckboxes.some((cb) => cb.checked);
  if (!anyPresetChecked && !lensCustomCheckbox.checked) {
    lensNoneCheckbox.checked = true;
  }
}

lensPresetCheckboxes.forEach((cb) => {
  cb.addEventListener("change", () => {
    if (cb.checked) clearNoneSelection();
    else reassertNoneIfNothingElseSelected();
  });
});

lensCustomCheckbox.addEventListener("change", () => {
  customLensText.hidden = !lensCustomCheckbox.checked;
  if (lensCustomCheckbox.checked) clearNoneSelection();
  else reassertNoneIfNothingElseSelected();
});

lensNoneCheckbox.addEventListener("change", () => {
  if (lensNoneCheckbox.checked) {
    lensPresetCheckboxes.forEach((cb) => (cb.checked = false));
    lensCustomCheckbox.checked = false;
    customLensText.hidden = true;
  } else {
    reassertNoneIfNothingElseSelected();
  }
});

mealTypeQuickPicks.addEventListener("click", (e) => {
  const btn = e.target.closest(".quick-pick");
  if (!btn) return;
  const type = btn.dataset.mealType;
  mealType = mealType === type ? "" : type;
  mealTypeQuickPicks.querySelectorAll(".quick-pick").forEach((b) => {
    b.classList.toggle("active", b === btn && mealType === type);
  });
});

cuisineQuickPicks.addEventListener("click", (e) => {
  const btn = e.target.closest(".quick-pick");
  if (!btn) return;
  const cuisine = btn.dataset.cuisine;
  cuisineInput.value = cuisineInput.value === cuisine ? "" : cuisine;
  cuisineQuickPicks.querySelectorAll(".quick-pick").forEach((b) => {
    b.classList.toggle("active", b === btn && cuisineInput.value === cuisine);
  });
});

// --- Mode toggle (tonight's dinner vs. meal prep for the week) ---------

function findRecipesIdleLabel() {
  return mode === "mealprep" ? "Plan my week" : "Find recipes";
}

function setMode(newMode) {
  mode = newMode;
  modeToggle.querySelectorAll(".mode-option").forEach((b) => {
    b.classList.toggle("active", b.dataset.mode === mode);
  });
  mealCountField.hidden = mode !== "mealprep";
  servingsLabel.textContent = mode === "mealprep" ? "Servings per meal" : "Servings";
  findRecipesBtn.textContent = findRecipesIdleLabel();
}

modeToggle.addEventListener("click", (e) => {
  const btn = e.target.closest(".mode-option");
  if (btn) setMode(btn.dataset.mode);
});

mealCountQuickPicks.addEventListener("click", (e) => {
  const btn = e.target.closest(".quick-pick");
  if (!btn) return;
  mealCountInput.value = btn.dataset.count;
  mealCountQuickPicks.querySelectorAll(".quick-pick").forEach((b) => {
    b.classList.toggle("active", b === btn);
  });
});

function composeIntakeMessage() {
  const lines = [];
  lines.push(`I have these ingredients: ${ingredients.join(", ")}.`);

  const selectedPresets = lensPresetCheckboxes.filter((cb) => cb.checked).map((cb) => cb.value);
  const customGoals = lensCustomCheckbox.checked ? customLensText.value.trim() : "";

  if (selectedPresets.length === 0 && !customGoals) {
    lines.push("Dietary lens: no restrictions.");
  } else if (selectedPresets.length > 0 && customGoals) {
    lines.push(
      `Dietary lens: ${selectedPresets.join(", ")}, plus these additional custom goals: ${customGoals}. ` +
        "All of these must be satisfied together, not just one -- save the custom part as a reusable custom lens if useful."
    );
  } else if (selectedPresets.length > 0) {
    lines.push(
      `Dietary lens: ${selectedPresets.join(", ")}.` +
        (selectedPresets.length > 1 ? " All of these must be satisfied together, not just one." : "")
    );
  } else {
    lines.push(`Dietary lens: no existing preset matches -- here are my goals: ${customGoals}. Save this as a custom lens if useful.`);
  }

  const servings = parseInt(servingsInput.value, 10);
  const servingsPhrase = `${servings > 0 ? servings : 1} serving${servings === 1 ? "" : "s"}`;

  const cuisine = cuisineInput.value.trim();
  if (cuisine) {
    lines.push(`Cuisine preference: ${cuisine}.`);
  }

  if (mealType) {
    lines.push(`Meal type: ${mealType.toLowerCase()}.`);
  }

  if (mode === "mealprep") {
    const mealCount = parseInt(mealCountInput.value, 10);
    const count = mealCount > 0 ? mealCount : 5;
    lines.push(
      `I'm cooking for ${servingsPhrase} per meal. This is for meal prep: please give me ` +
        `${count} distinct recipes for the week, not just one meal for tonight. Favor recipes ` +
        `that share overlapping ingredients so what I have gets used efficiently with little waste, ` +
        `and note briefly how each one stores and reheats.`
    );
  } else {
    lines.push(`I'm cooking for ${servingsPhrase}.`);
    lines.push("Please suggest recipe options.");
  }
  return lines.join(" ");
}

findRecipesBtn.addEventListener("click", async () => {
  if (ingredients.length === 0) return;
  intakeErrorEl.hidden = true;
  findRecipesBtn.disabled = true;
  findRecipesBtn.textContent = mode === "mealprep" ? "Planning your week..." : "Finding recipes...";

  const message = composeIntakeMessage();
  saveProfile(); // fire-and-forget -- remembers servings/cuisine for next visit
  try {
    await createSession();
    intakeEl.hidden = true;
    chatEl.hidden = false;
    composerEl.hidden = false;
    resetChat();
    await sendMessage(message, { showUserBubble: false });
  } catch (err) {
    intakeErrorEl.textContent = "Couldn't reach PantryLens: " + err.message;
    intakeErrorEl.hidden = false;
  } finally {
    findRecipesBtn.disabled = ingredients.length === 0;
    findRecipesBtn.textContent = findRecipesIdleLabel();
  }
});

// --- Chat rendering ----------------------------------------------------

function scrollToBottom() {
  chatEl.scrollTop = chatEl.scrollHeight;
}

function addBubble(role, text) {
  const div = document.createElement("div");
  div.className = "msg " + role;
  div.textContent = text;
  chatEl.appendChild(div);
  scrollToBottom();
  return div;
}

function addToolChip(label, variant) {
  const div = document.createElement("div");
  div.className = "tool-chip" + (variant ? " " + variant : "");
  div.textContent = label;
  chatEl.appendChild(div);
  scrollToBottom();
  return div;
}

function macroChip(value, unit, label) {
  if (value === undefined || value === null) return null;
  const span = document.createElement("span");
  span.className = "macro-chip";
  span.textContent = `${value}${unit} ${label}`;
  return span;
}

function addRecipeCard(args) {
  const card = document.createElement("div");
  card.className = "recipe-card";

  const header = document.createElement("div");
  header.className = "recipe-card-header";
  const title = document.createElement("h3");
  title.textContent = args.title || "Recipe";
  header.appendChild(title);
  if (args.cuisine) {
    const badge = document.createElement("span");
    badge.className = "cuisine-badge";
    badge.textContent = args.cuisine;
    header.appendChild(badge);
  }
  if (args.servings) {
    const badge = document.createElement("span");
    badge.className = "cuisine-badge";
    badge.textContent = `${args.servings} serving${args.servings === 1 ? "" : "s"}`;
    header.appendChild(badge);
  }
  card.appendChild(header);

  const macroRow = document.createElement("div");
  macroRow.className = "macro-row";
  const macros = [
    macroChip(args.calories, "", "kcal"),
    macroChip(args.proteinG, "g", "protein"),
    macroChip(args.carbsG, "g", "carbs"),
    macroChip(args.fatG, "g", "fat"),
  ].filter(Boolean);
  macros.forEach((m) => macroRow.appendChild(m));
  if (macros.length) card.appendChild(macroRow);

  // additionalIngredients (see ../../tools_adk.go's proposeRecipeArgs) are
  // entries copied verbatim from `ingredients` for anything the user didn't
  // say they have -- surfaced two ways: a summary line here so it's visible
  // without expanding the details below, and a "need to buy" tag inline on
  // the matching <li> once they do.
  const additionalSet = new Set(Array.isArray(args.additionalIngredients) ? args.additionalIngredients : []);
  if (additionalSet.size) {
    const note = document.createElement("p");
    note.className = "shopping-note";
    note.textContent = "🛒 You'll need to pick up: " + Array.from(additionalSet).join(", ");
    card.appendChild(note);
  }

  const details = document.createElement("details");
  const summary = document.createElement("summary");
  summary.textContent = "Ingredients & steps";
  details.appendChild(summary);

  const body = document.createElement("div");
  body.className = "recipe-body";
  if (Array.isArray(args.ingredients) && args.ingredients.length) {
    const h = document.createElement("h4");
    h.textContent = "Ingredients";
    const ul = document.createElement("ul");
    args.ingredients.forEach((i) => {
      const li = document.createElement("li");
      li.appendChild(document.createTextNode(i));
      if (additionalSet.has(i)) {
        li.className = "need-to-buy";
        const tag = document.createElement("span");
        tag.className = "need-to-buy-tag";
        tag.textContent = "need to buy";
        li.appendChild(document.createTextNode(" "));
        li.appendChild(tag);
      }
      ul.appendChild(li);
    });
    body.appendChild(h);
    body.appendChild(ul);
  }
  if (Array.isArray(args.steps) && args.steps.length) {
    const h = document.createElement("h4");
    h.textContent = "Steps";
    const ol = document.createElement("ol");
    args.steps.forEach((s) => {
      const li = document.createElement("li");
      li.textContent = s;
      ol.appendChild(li);
    });
    body.appendChild(h);
    body.appendChild(ol);
  }
  details.appendChild(body);
  card.appendChild(details);

  if (args.lensNote) {
    const note = document.createElement("p");
    note.className = "lens-note";
    note.textContent = "✓ " + args.lensNote;
    card.appendChild(note);
  }

  if (args.storageNote) {
    const note = document.createElement("p");
    note.className = "storage-note";
    note.textContent = "🧊 " + args.storageNote;
    card.appendChild(note);
  }

  // args's fields already match POST /recipes's body shape exactly (see
  // ../frontend.go's saveRecipeBody) -- propose_recipe's args ARE a saved
  // recipe's shape (plus userId), so no reshaping is needed here. Saving
  // does NOT navigate anywhere on its own -- it just stores the recipe and
  // confirms inline; "My saved recipes" (see openMyRecipes below) is where
  // you'd deliberately choose to open one.
  const saveRow = document.createElement("div");
  saveRow.className = "save-recipe-row";
  const saveBtn = document.createElement("button");
  saveBtn.type = "button";
  saveBtn.className = "export-btn";
  saveBtn.textContent = "Save recipe";
  saveBtn.addEventListener("click", async () => {
    saveBtn.disabled = true;
    saveBtn.textContent = "Saving...";
    try {
      const res = await fetch("/recipes", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...args, userId, mealPrepBatch: args.storageNote ? currentMealPrepBatchLabel() : "" }),
      });
      if (!res.ok) {
        const body = await res.text().catch(() => "");
        throw new Error(`${res.status} ${res.statusText}: ${body}`);
      }
      const data = await res.json();
      saveBtn.textContent = "✓ Saved";
      const viewLink = document.createElement("a");
      viewLink.href = data.url;
      viewLink.className = "view-saved-link";
      viewLink.textContent = "View";
      saveRow.appendChild(viewLink);
    } catch (err) {
      saveBtn.disabled = false;
      saveBtn.textContent = "Couldn't save -- try again";
    }
  });
  saveRow.appendChild(saveBtn);
  card.appendChild(saveRow);

  chatEl.appendChild(card);
  scrollToBottom();
}

// addSaveAllRow renders a one-click "save every recipe from this batch"
// action under a meal-prep response's cards -- saving one at a time via
// each card's own button still works, this is just the convenience path
// for the common case of wanting the whole week. recipes should already be
// filtered to the ones carrying a storageNote (see the sendMessage call
// site); all of them get the same currentMealPrepBatchLabel() so they
// group together in "My saved recipes".
function addSaveAllRow(recipes) {
  const row = document.createElement("div");
  row.className = "save-all-row";
  const btn = document.createElement("button");
  btn.type = "button";
  btn.className = "save-all-btn";
  btn.textContent = `💾 Save all ${recipes.length} recipes`;
  btn.addEventListener("click", async () => {
    btn.disabled = true;
    btn.textContent = "Saving...";
    try {
      const batchLabel = currentMealPrepBatchLabel();
      const results = await Promise.all(
        recipes.map((r) =>
          fetch("/recipes", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ ...r, userId, mealPrepBatch: batchLabel }),
          })
        )
      );
      const failed = results.filter((res) => !res.ok).length;
      btn.textContent =
        failed > 0
          ? `Saved ${recipes.length - failed}/${recipes.length} -- ${failed} failed`
          : `✓ Saved all ${recipes.length} recipes`;
    } catch (err) {
      btn.disabled = false;
      btn.textContent = "Couldn't save -- try again";
      return;
    }
  });
  row.appendChild(btn);
  chatEl.appendChild(row);
  scrollToBottom();
}

let typingEl = null;
function showTyping() {
  if (typingEl) return;
  typingEl = document.createElement("div");
  typingEl.className = "typing";
  typingEl.textContent = "PantryLens is thinking...";
  chatEl.appendChild(typingEl);
  scrollToBottom();
}
function hideTyping() {
  if (typingEl) {
    typingEl.remove();
    typingEl = null;
  }
}

async function api(path, options) {
  const res = await fetch("/api" + path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`${res.status} ${res.statusText}: ${body}`);
  }
  return res.status === 204 ? null : res.json();
}

async function createSession() {
  const session = await api(`/apps/${APP_NAME}/users/${userId}/sessions`, {
    method: "POST",
    body: "{}",
  });
  sessionId = session.id;
  localStorage.setItem("pantrylens_session_id", sessionId);
}

function resetChat() {
  chatEl.innerHTML = "";
}

// --- Resume last session on page load ------------------------------------
//
// Refreshing the tab used to always lose your place -- sessionId only ever
// lived in a JS variable, reset on every load. Now the last active session
// ID is remembered in localStorage (see createSession above) and, on load,
// checked against GET /apps/{app}/users/{user}/sessions/{sessionId} --
// that returns the session's full event history in the exact same shape
// /run does, so the existing renderEvent() replays it directly, no new
// rendering logic needed. This only survives as long as the server
// process does, though -- ADK's default session service is in-memory, so
// a server restart still loses it; true persistence across restarts, plus
// a "past conversations" list, is a bigger separate piece this quick fix
// deliberately doesn't cover.
async function resumeSession() {
  const storedSessionId = localStorage.getItem("pantrylens_session_id");
  if (!storedSessionId) return false;

  try {
    const session = await api(`/apps/${APP_NAME}/users/${userId}/sessions/${storedSessionId}`, {
      method: "GET",
    });
    if (!Array.isArray(session.events) || session.events.length === 0) {
      // Created but nothing was ever sent to it -- not worth resuming into.
      localStorage.removeItem("pantrylens_session_id");
      return false;
    }
    sessionId = session.id;
    intakeEl.hidden = true;
    chatEl.hidden = false;
    composerEl.hidden = false;
    resetChat();
    session.events.forEach(renderEvent);
    return true;
  } catch (err) {
    // Most likely the session no longer exists (server restarted since --
    // see the comment above) -- fall back to the intake form rather than
    // surfacing an error for something this routine.
    localStorage.removeItem("pantrylens_session_id");
    return false;
  }
}

async function startNewSession() {
  ingredients = [];
  mealPrepBatchLabel = "";
  mealType = "";
  renderIngredientChips();
  cuisineInput.value = "";
  servingsInput.value = "2";
  mealCountInput.value = "5";
  cuisineQuickPicks.querySelectorAll(".quick-pick").forEach((b) => b.classList.remove("active"));
  mealCountQuickPicks.querySelectorAll(".quick-pick").forEach((b) => b.classList.remove("active"));
  mealTypeQuickPicks.querySelectorAll(".quick-pick").forEach((b) => b.classList.remove("active"));
  setMode("tonight");
  customLensText.value = "";
  customLensText.hidden = true;
  lensPresetCheckboxes.forEach((cb) => (cb.checked = false));
  lensCustomCheckbox.checked = false;
  lensNoneCheckbox.checked = true;

  resetChat();
  sessionId = null;
  localStorage.removeItem("pantrylens_session_id");
  chatEl.hidden = true;
  composerEl.hidden = true;
  myRecipesEl.hidden = true;
  intakeEl.hidden = false;
  intakeErrorEl.hidden = true;
  loadProfile(); // re-prefill servings/cuisine from what was last saved
}

// --- My saved recipes ----------------------------------------------------
//
// GET /recipes?userId=... (see ../frontend.go) -- a plain listing scoped to
// this browser's anonymous userId, most recently saved first. Opening it
// hides whichever of intake/chat was showing and remembers which one, so
// "Back" restores it rather than always dumping back to the intake form.

let viewBeforeMyRecipes = "intake"; // "intake" | "chat"
let savedRecipesCache = []; // last fetch's full list, so tab filtering doesn't refetch
let savedRecipesFilter = ""; // "" (All) | "Breakfast" | "Lunch" | "Dinner" | "Snack"

async function openMyRecipes() {
  viewBeforeMyRecipes = intakeEl.hidden ? "chat" : "intake";
  intakeEl.hidden = true;
  chatEl.hidden = true;
  composerEl.hidden = true;
  myRecipesEl.hidden = false;

  savedRecipesFilter = "";
  myRecipesTabsEl.querySelectorAll(".quick-pick").forEach((b) => {
    b.classList.toggle("active", b.dataset.mealTypeFilter === "");
  });

  myRecipesListEl.textContent = "Loading...";
  try {
    const res = await fetch(`/recipes?userId=${encodeURIComponent(userId)}`);
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    const data = await res.json();
    savedRecipesCache = data.recipes || [];
    renderFilteredSavedRecipes();
  } catch (err) {
    myRecipesListEl.textContent = "Couldn't load your saved recipes: " + err.message;
  }
}

function renderFilteredSavedRecipes() {
  const filtered = savedRecipesFilter
    ? savedRecipesCache.filter((r) => r.mealType === savedRecipesFilter)
    : savedRecipesCache;
  renderSavedRecipesList(filtered, savedRecipesFilter);
}

myRecipesTabsEl.addEventListener("click", (e) => {
  const btn = e.target.closest(".quick-pick");
  if (!btn) return;
  savedRecipesFilter = btn.dataset.mealTypeFilter;
  myRecipesTabsEl.querySelectorAll(".quick-pick").forEach((b) => b.classList.toggle("active", b === btn));
  renderFilteredSavedRecipes();
});

function formatSavedDate(iso) {
  if (!iso) return "";
  try {
    return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
  } catch (err) {
    return "";
  }
}

// renderSavedRecipesList groups recipes by mealPrepBatch (see
// core.SavedRecipe.MealPrepBatch), in first-seen order -- recipes already
// arrive most-recently-saved first (see RecipeStore.List), so a batch's
// group lands wherever its most recent member would, with plain
// (non-batch) recipes staying flat exactly as before. activeFilter is only
// used to word the empty state -- filtering itself already happened in
// renderFilteredSavedRecipes before recipes got here.
function renderSavedRecipesList(recipes, activeFilter) {
  myRecipesListEl.innerHTML = "";
  if (recipes.length === 0) {
    const empty = document.createElement("p");
    empty.className = "saved-recipes-empty";
    empty.textContent = activeFilter
      ? `No ${activeFilter.toLowerCase()} recipes saved yet.`
      : 'You haven\'t saved any recipes yet -- hit "Save recipe" on a card to add one here.';
    myRecipesListEl.appendChild(empty);
    return;
  }

  const groups = new Map(); // batch label ("" = ungrouped) -> recipes, first-seen order
  recipes.forEach((r) => {
    const key = r.mealPrepBatch || "";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(r);
  });

  const list = document.createElement("div");
  list.className = "saved-recipes-list";
  groups.forEach((items, batchLabel) => {
    if (batchLabel) {
      const header = document.createElement("div");
      header.className = "saved-recipes-batch-header";
      header.textContent = `🍱 ${batchLabel}`;
      list.appendChild(header);
    }
    items.forEach((r) => {
      const item = document.createElement("a");
      item.className = "saved-recipe-item" + (batchLabel ? " saved-recipe-item-grouped" : "");
      item.href = r.url;
      const title = document.createElement("span");
      title.className = "saved-recipe-title";
      title.textContent = r.title || "Untitled recipe";
      const date = document.createElement("span");
      date.className = "saved-recipe-date";
      date.textContent = formatSavedDate(r.savedAt);
      item.appendChild(title);
      item.appendChild(date);
      list.appendChild(item);
    });
  });
  myRecipesListEl.appendChild(list);
}

function closeMyRecipes() {
  myRecipesEl.hidden = true;
  if (viewBeforeMyRecipes === "chat") {
    chatEl.hidden = false;
    composerEl.hidden = false;
  } else {
    intakeEl.hidden = false;
  }
}

myRecipesBtn.addEventListener("click", openMyRecipes);
myRecipesBackBtn.addEventListener("click", closeMyRecipes);

function renderEvent(event) {
  const parts = event.content && event.content.parts ? event.content.parts : [];
  for (const part of parts) {
    if (part.functionCall) {
      if (part.functionCall.name === "propose_recipe") {
        addRecipeCard(part.functionCall.args || {});
      } else {
        const label = TOOL_LABELS[part.functionCall.name] || `🔧 ${part.functionCall.name}`;
        addToolChip(label);
      }
    }
    if (part.functionResponse) {
      const name = part.functionResponse.name;
      const response = part.functionResponse.response || {};
      if (name === "check_recipe_against_lens") {
        if (response.compliant === true) {
          addToolChip("✅ Compliant with your lens", "compliant");
        } else if (response.compliant === false) {
          const n = Array.isArray(response.violations) ? response.violations.length : 0;
          addToolChip(`⚠️ ${n} issue${n === 1 ? "" : "s"} found -- revising`, "violation");
        }
      }
      // propose_recipe's own response is a trivial ack -- nothing to render.
      // Saving a recipe (see addRecipeCard's "Save & view" button) is a
      // direct POST /recipes call, not a tool response -- nothing to
      // render here for that either.
    }
    if (part.text && part.text.trim() && event.author !== "user") {
      addBubble("assistant", part.text);
    }
  }
}

async function sendMessage(text, opts) {
  const showUserBubble = !opts || opts.showUserBubble !== false;
  if (showUserBubble) addBubble("user", text);
  inputEl.disabled = true;
  sendBtn.disabled = true;
  showTyping();

  try {
    if (!sessionId) {
      await createSession();
    }
    const events = await api("/run", {
      method: "POST",
      body: JSON.stringify({
        appName: APP_NAME,
        userId,
        sessionId,
        newMessage: { role: "user", parts: [{ text }] },
      }),
    });
    hideTyping();
    const proposedThisTurn = [];
    for (const event of events || []) {
      renderEvent(event);
      for (const part of (event.content && event.content.parts) || []) {
        if (part.functionCall && part.functionCall.name === "propose_recipe") {
          proposedThisTurn.push(part.functionCall.args || {});
        }
      }
    }
    // Driven by whether the agent actually set storageNote (see
    // propose_recipe's field of the same name), not by the intake form's
    // mode toggle -- a meal-prep request typed straight into the composer
    // mid-conversation, never touching the toggle, still gets this.
    const mealPrepRecipesThisTurn = proposedThisTurn.filter((r) => r.storageNote);
    if (mealPrepRecipesThisTurn.length > 0) {
      addSaveAllRow(mealPrepRecipesThisTurn);
    }
  } catch (err) {
    hideTyping();
    addBubble("error", "Something went wrong: " + err.message);
  } finally {
    inputEl.disabled = false;
    sendBtn.disabled = false;
    inputEl.focus();
  }
}

composerEl.addEventListener("submit", (e) => {
  e.preventDefault();
  const text = inputEl.value.trim();
  if (!text) return;
  inputEl.value = "";
  sendMessage(text);
});

newSessionBtn.addEventListener("click", startNewSession);

// --- Preferences (servings/cuisine prefill) -----------------------------
//
// Deliberately NOT under /api/... -- that prefix belongs to ADK's own REST
// API (see the file header), and this is a plain PantryLens-owned endpoint
// (see ../frontend.go) that has nothing to do with the agent or a chat
// session; there's no session to attach it to yet when the page first
// loads.

async function loadProfile() {
  try {
    const res = await fetch(`/profile/${userId}`);
    if (!res.ok) return;
    const prefs = await res.json();
    if (prefs.lastServings) servingsInput.value = String(prefs.lastServings);
    if (prefs.lastCuisine) {
      cuisineInput.value = prefs.lastCuisine;
      cuisineQuickPicks.querySelectorAll(".quick-pick").forEach((b) => {
        b.classList.toggle("active", b.dataset.cuisine === prefs.lastCuisine);
      });
    }
  } catch (err) {
    // Non-fatal -- prefill is a convenience, not required to use the app.
  }
}

function saveProfile() {
  const servings = parseInt(servingsInput.value, 10);
  fetch(`/profile/${userId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      lastServings: servings > 0 ? servings : 0,
      lastCuisine: cuisineInput.value.trim(),
    }),
  }).catch(() => {}); // fire-and-forget -- not required to use the app
}

renderIngredientChips();
loadProfile();
resumeSession();
