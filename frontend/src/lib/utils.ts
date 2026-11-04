import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) { return twMerge(clsx(inputs)) }

export function readPreference(key: string, fallback: string): string {
  try { return localStorage.getItem(key) ?? fallback } catch { return fallback }
}

export function savePreference(key: string, value: string) {
  try { localStorage.setItem(key, value) } catch { /* The UI remains usable when storage is unavailable. */ }
}
