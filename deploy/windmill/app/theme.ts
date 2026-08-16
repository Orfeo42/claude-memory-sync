const TOKENS = [
  "border-accent",
  "border-light",
  "border-normal",
  "border-selected",
  "component-button-accent-secondary",
  "surface-accent-hover",
  "surface-accent-primary",
  "surface-accent-selected",
  "surface-hover",
  "surface-input",
  "surface-primary",
  "surface-secondary",
  "surface-selected",
  "surface-sunken",
  "surface-tertiary",
  "text-emphasis",
  "text-hint",
  "text-primary",
  "text-secondary",
] as const;

export const startThemeSync = (): void => {
  let parentRoot: HTMLElement;
  try {
    parentRoot = window.parent.document.documentElement;
  } catch {
    return;
  }

  const apply = (): void => {
    const parentStyles = getComputedStyle(parentRoot);
    const root = document.documentElement;
    for (const token of TOKENS) {
      const value = parentStyles.getPropertyValue(`--color-${token}`).trim();
      if (value !== "") {
        root.style.setProperty(`--color-${token}`, value);
      }
    }
    root.classList.toggle(
      "windmill-dark",
      parentRoot.classList.contains("dark"),
    );
  };

  apply();
  new MutationObserver(apply).observe(parentRoot, {
    attributes: true,
    attributeFilter: ["class", "style"],
  });
};
