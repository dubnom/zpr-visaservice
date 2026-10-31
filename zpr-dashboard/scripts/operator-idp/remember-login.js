(() => {
  const login = document.getElementById("login");
  const password = document.getElementById("password");
  const form = login?.form;
  if (!form || !password || password.form !== form) return;

  const key = "zpr.operator-idp.remembered-login";
  const preferenceKey = `${key}.enabled`;
  login.autocomplete = "username";
  password.autocomplete = "current-password";
  const row = document.createElement("div");
  row.className = "theme-form-row";
  const label = document.createElement("label");
  const remember = document.createElement("input");
  remember.type = "checkbox";
  remember.id = "remember-login";
  remember.checked = true;
  label.append(remember, " Remember email or username on this device");
  const status = document.createElement("p");
  status.setAttribute("role", "status");
  row.append(label, status);
  form.insertBefore(row, form.querySelector("button[type='submit']"));

  function storageFailure() {
    status.textContent = "Your browser could not access the remembered email or username. Sign-in is still available.";
  }

  try {
    remember.checked = localStorage.getItem(preferenceKey) !== "false";
    const saved = localStorage.getItem(key);
    if (remember.checked && saved && !login.value) {
      login.value = saved;
      if (document.activeElement === login || login.hasAttribute("autofocus")) password.focus();
    }
  } catch {
    storageFailure();
  }

  remember.addEventListener("change", () => {
    try {
      if (remember.checked) localStorage.removeItem(preferenceKey);
      else {
        localStorage.removeItem(key);
        localStorage.setItem(preferenceKey, "false");
      }
      status.textContent = "";
    } catch {
      storageFailure();
    }
  });
  form.addEventListener("submit", () => {
    try {
      if (remember.checked && login.value.trim()) localStorage.setItem(key, login.value.trim());
      else localStorage.removeItem(key);
      status.textContent = "";
    } catch {
      storageFailure();
    }
  });
})();
