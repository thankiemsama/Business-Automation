const OFFLINE_CACHE = "nexa-offline-v1";
self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(OFFLINE_CACHE).then((cache) => cache.add("/offline.html")),
  );
});
self.addEventListener("activate", (event) => {
  event.waitUntil(
    Promise.all([
      self.clients.claim(),
      caches
        .keys()
        .then((keys) =>
          Promise.all(
            keys
              .filter(
                (key) =>
                  key.startsWith("nexa-offline-") && key !== OFFLINE_CACHE,
              )
              .map((key) => caches.delete(key)),
          ),
        ),
    ]),
  );
});
self.addEventListener("fetch", (event) => {
  if (
    event.request.mode !== "navigate" ||
    new URL(event.request.url).origin !== self.location.origin || new URL(event.request.url).pathname.startsWith("/api/")
  )
    return;
  event.respondWith(
    fetch(event.request).catch(
      async () => (await caches.match("/offline.html")) || Response.error(),
    ),
  );
});

self.addEventListener("push", (event) => {
  event.waitUntil(
    self.registration.showNotification("Nexa", {
      body: event.data?.text() || "Bạn có thông báo mới.",
      tag: "nexa-workspace",
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(self.clients.openWindow("/"));
});
