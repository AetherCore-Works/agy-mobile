self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('push', (event) => {
  let payload = {
    title: 'AGY Mobile Gateway',
    body: 'Nova notificação recebida.',
    url: '/',
    tag: 'agy-gateway-alert',
  };

  if (event.data) {
    try {
      const parsed = event.data.json();
      if (parsed && typeof parsed === 'object') {
        payload = {
          ...payload,
          ...parsed,
          title: (parsed.title || payload.title).toString(),
          body: (parsed.body || payload.body).toString(),
        };
      }
    } catch {
      const txt = event.data.text();
      if (txt) payload.body = txt;
    }
  }

  const options = {
    body: payload.body,
    icon: '/static/icon-192.png',
    badge: '/static/icon-192.png',
    tag: payload.tag || 'agy-gateway-alert',
    data: { url: payload.url || '/' },
  };

  event.waitUntil(self.registration.showNotification(payload.title, options));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const rawUrl = (event.notification.data && event.notification.data.url) || '/';
  const targetUrl = new URL(rawUrl, self.location.origin).href;

  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windowClients) => {
      for (const client of windowClients) {
        if (client && 'focus' in client) {
          return client.focus().then(() => {
            if ('navigate' in client && rawUrl && rawUrl !== '/') {
              return client.navigate(targetUrl).catch(() => client);
            }
            return client;
          });
        }
      }
      if (self.clients.openWindow) {
        return self.clients.openWindow(targetUrl);
      }
      return undefined;
    })
  );
});

self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(
    (async () => {
      try {
        const res = await fetch('/api/push/vapid-public-key');
        if (!res.ok) return;
        const data = await res.json();
        if (!data || !data.public_key) return;
        const padding = '='.repeat((4 - (data.public_key.length % 4)) % 4);
        const base64 = (data.public_key + padding).replace(/-/g, '+').replace(/_/g, '/');
        const raw = atob(base64);
        const key = new Uint8Array(raw.length);
        for (let i = 0; i < raw.length; i++) key[i] = raw.charCodeAt(i);
        const newSub = await self.registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: key,
        });
        await fetch('/api/push/subscribe', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(newSub.toJSON()),
        });
      } catch {
        // best-effort renewal for mobile APNs/FCM rotation
      }
    })()
  );
});
