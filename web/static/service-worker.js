// 缓存名称和版本
const CACHE_NAME = 'kompanion-cache-v1';
const ASSET_URLS = [
  '/',
  '/static/static.css',
  '/static/monospace.css',
  '/static/manifest.json',
  '/static/icons/icon.svg'
];

// 安装Service Worker并缓存静态资源
self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => {
        return cache.addAll(ASSET_URLS);
      })
      .then(() => self.skipWaiting())
  );
});

// 激活Service Worker并清理旧缓存
self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(cacheNames => {
      return Promise.all(
        cacheNames.map(cacheName => {
          if (cacheName !== CACHE_NAME) {
            return caches.delete(cacheName);
          }
        })
      );
    }).then(() => self.clients.claim())
  );
});

// 拦截网络请求并提供缓存响应
self.addEventListener('fetch', event => {
  event.respondWith(
    caches.match(event.request)
      .then(response => {
        // 如果缓存中有匹配的资源，则返回缓存的资源
        if (response) {
          return response;
        }
        // 否则，发起网络请求
        return fetch(event.request).catch(() => {
          // 如果网络请求失败，可以返回一个备用页面
          if (event.request.mode === 'navigate') {
            return caches.match('/');
          }
        });
      })
  );
});