/*! coi-serviceworker v0.1.7 - Guido Zuidhof and contributors, licensed under MIT */
/*
 * This is a Service Worker that enables Cross-Origin Isolation (COI) for GitHub Pages
 * and other static hosting that doesn't allow setting HTTP headers.
 * 
 * It intercepts requests and adds the necessary COOP and COEP headers to enable
 * SharedArrayBuffer, which is required for OPFS in SQLite WASM.
 */

if (typeof window === 'undefined') {
    self.addEventListener("install", () => self.skipWaiting());
    self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

    self.addEventListener("fetch", function (event) {
        if (event.request.cache === "only-if-cached" && event.request.mode !== "same-origin") {
            return;
        }
        
        event.respondWith(
            fetch(event.request)
                .then((response) => {
                    if (response.status === 0) {
                        return response;
                    }

                    const newHeaders = new Headers(response.headers);
                    newHeaders.set("Cross-Origin-Embedder-Policy", "require-corp");
                    newHeaders.set("Cross-Origin-Opener-Policy", "same-origin");

                    return new Response(response.body, {
                        status: response.status,
                        statusText: response.statusText,
                        headers: newHeaders,
                    });
                })
                .catch((e) => console.error(e))
        );
    });
} else {
    // If we're in the main thread, register the service worker
    (() => {
        // You can customize the registration scope here
        const reloadedBySelf = window.sessionStorage.getItem("coiReloadedBySelf");
        window.sessionStorage.removeItem("coiReloadedBySelf");
        
        const coiRequested = () => {
            const params = new URLSearchParams(window.location.search);
            return params.has("coi");
        };

        if (reloadedBySelf === "true" || coiRequested()) {
            // We're already isolated or explicitly requested COI
            return;
        }

        // Register the service worker
        if (window.crossOriginIsolated !== true) {
            window.sessionStorage.setItem("coiReloadedBySelf", "true");
            
            if ("serviceWorker" in navigator) {
                navigator.serviceWorker
                    .register(window.location.pathname + "coi-serviceworker.js")
                    .then(() => {
                        window.location.reload();
                    })
                    .catch((error) => {
                        console.error("Service Worker registration failed:", error);
                    });
            }
        }
    })();
}