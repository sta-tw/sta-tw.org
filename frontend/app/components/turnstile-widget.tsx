"use client";

import Script from "next/script";
import { useEffect, useRef, useState } from "react";

const siteKey = "0x4AAAAAAFEeQVe0smpckO5a";

type TurnstileAPI = {
    render: (
        element: HTMLElement,
        options: {
            sitekey: string;
            action: string;
            theme: "light" | "dark" | "auto";
            size: "normal" | "compact" | "flexible";
            appearance: "always" | "execute" | "interaction-only";
            callback: (token: string) => void;
            "expired-callback": () => void;
            "error-callback": () => void;
        }
    ) => string;
    reset: (widgetId: string) => void;
    remove: (widgetId: string) => void;
};

declare global {
    interface Window {
        turnstile?: TurnstileAPI;
    }
}

type Props = {
    action: "login" | "signup";
    onTokenChange: (token: string) => void;
    resetSignal: number;
};

export default function TurnstileWidget({ action, onTokenChange, resetSignal }: Props) {
    const containerRef = useRef<HTMLDivElement>(null);
    const widgetIdRef = useRef<string | null>(null);
    const callbackRef = useRef(onTokenChange);
    const [scriptReady, setScriptReady] = useState(false);
    const [scriptError, setScriptError] = useState(false);

    useEffect(() => {
        callbackRef.current = onTokenChange;
    }, [onTokenChange]);

    useEffect(() => {
        if (!scriptReady || !window.turnstile || !containerRef.current || widgetIdRef.current) return;
        widgetIdRef.current = window.turnstile.render(containerRef.current, {
            sitekey: siteKey,
            action,
            theme: "light",
            size: "flexible",
            appearance: "interaction-only",
            callback: (token) => callbackRef.current(token),
            "expired-callback": () => callbackRef.current(""),
            "error-callback": () => callbackRef.current("")
        });
        return () => {
            if (widgetIdRef.current) window.turnstile?.remove(widgetIdRef.current);
            widgetIdRef.current = null;
        };
    }, [action, scriptReady]);

    useEffect(() => {
        if (resetSignal > 0 && widgetIdRef.current) {
            callbackRef.current("");
            window.turnstile?.reset(widgetIdRef.current);
        }
    }, [resetSignal]);

    return (
        <div className="flex flex-col items-center justify-center [&:has(iframe)]:mt-5">
            <Script
                src="https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit"
                strategy="afterInteractive"
                onReady={() => setScriptReady(true)}
                onError={() => setScriptError(true)}
            />
            <div ref={containerRef} />
            {scriptError ? (
                <p className="text-sm text-red-600">驗證元件載入失敗，請重新整理頁面。</p>
            ) : null}
        </div>
    );
}
