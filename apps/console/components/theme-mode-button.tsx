"use client";

import { ComputerDesktopIcon, MoonIcon, SunIcon } from "@heroicons/react/24/outline";
import { useEffect, useMemo, useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";
import {
  getNextThemeMode,
  isThemeMode,
  resolveThemeMode,
  THEME_STORAGE_KEY,
  type ThemeMode,
} from "@/lib/theme";

const modeLabel: Record<ThemeMode, string> = {
  light: "浅色",
  dark: "深色",
  system: "跟随系统",
};
const THEME_MODE_CHANGE_EVENT = "sysarmor-manager-theme-mode-change";

function getStoredMode(): ThemeMode {
  if (typeof window === "undefined") return "system";

  const storedMode = window.localStorage.getItem(THEME_STORAGE_KEY);
  return storedMode && isThemeMode(storedMode) ? storedMode : "system";
}

function applyThemeMode(mode: ThemeMode) {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  const resolved = resolveThemeMode(mode, media.matches);

  document.documentElement.classList.toggle("dark", resolved === "dark");
  document.documentElement.dataset.theme = mode;
}

function subscribeThemeMode(onStoreChange: () => void) {
  window.addEventListener("storage", onStoreChange);
  window.addEventListener(THEME_MODE_CHANGE_EVENT, onStoreChange);

  return () => {
    window.removeEventListener("storage", onStoreChange);
    window.removeEventListener(THEME_MODE_CHANGE_EVENT, onStoreChange);
  };
}

function getServerThemeMode(): ThemeMode {
  return "system";
}

export function ThemeModeButton() {
  const mode = useSyncExternalStore(subscribeThemeMode, getStoredMode, getServerThemeMode);

  useEffect(() => {
    applyThemeMode(mode);
  }, [mode]);

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const handleChange = () => applyThemeMode(mode);

    media.addEventListener("change", handleChange);
    return () => media.removeEventListener("change", handleChange);
  }, [mode]);

  const Icon = useMemo(() => {
    if (mode === "light") return SunIcon;
    if (mode === "dark") return MoonIcon;
    return ComputerDesktopIcon;
  }, [mode]);

  function cycleThemeMode() {
    const nextMode = getNextThemeMode(mode);
    window.localStorage.setItem(THEME_STORAGE_KEY, nextMode);
    applyThemeMode(nextMode);
    window.dispatchEvent(new Event(THEME_MODE_CHANGE_EVENT));
  }

  return (
    <Button
      aria-label={`主题：${modeLabel[mode]}，点击切换`}
      intent="plain"
      size="sq-sm"
      onPress={cycleThemeMode}
    >
      <Icon />
    </Button>
  );
}
