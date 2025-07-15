// AI SA Assistant Service Worker
// Provides offline functionality and caching for mobile users

const CACHE_NAME = 'ai-sa-assistant-v1';
const STATIC_CACHE_NAME = 'ai-sa-assistant-static-v1';
const DATA_CACHE_NAME = 'ai-sa-assistant-data-v1';

// Static assets to cache for offline use
const STATIC_FILES = [
    '/',
    '/static/style.css',
    '/static/app.js',
    '/static/manifest.json',
    '/static/libs/mermaid.min.js',
    '/static/libs/prism.min.js',
    '/static/libs/prism-autoloader.min.js',
    '/static/libs/prism-copy-to-clipboard.min.js',
    '/static/libs/prism.css'
];

// API endpoints to cache for offline access (for future use)
const _API_ENDPOINTS = ['/conversations', '/health'];

// Install event - cache static assets
self.addEventListener('install', event => {
    console.log('SA Assistant SW: Installing...');

    event.waitUntil(
        caches
            .open(STATIC_CACHE_NAME)
            .then(cache => {
                console.log('SA Assistant SW: Caching static files');
                return cache.addAll(STATIC_FILES);
            })
            .then(() => {
                console.log('SA Assistant SW: Static files cached successfully');
                return self.skipWaiting();
            })
            .catch(error => {
                console.error('SA Assistant SW: Failed to cache static files', error);
            })
    );
});

// Activate event - clean up old caches
self.addEventListener('activate', event => {
    console.log('SA Assistant SW: Activating...');

    event.waitUntil(
        caches
            .keys()
            .then(cacheNames => {
                return Promise.all(
                    cacheNames.map(cacheName => {
                        if (
                            cacheName !== STATIC_CACHE_NAME &&
                            cacheName !== DATA_CACHE_NAME &&
                            cacheName !== CACHE_NAME
                        ) {
                            console.log('SA Assistant SW: Deleting old cache', cacheName);
                            return caches.delete(cacheName);
                        }
                        return Promise.resolve();
                    })
                );
            })
            .then(() => {
                console.log('SA Assistant SW: Activated successfully');
                return self.clients.claim();
            })
    );
});

// Fetch event - implement cache-first strategy for static assets
self.addEventListener('fetch', event => {
    const { request } = event;
    const url = new URL(request.url);

    // Handle static assets with cache-first strategy
    if (
        request.destination === 'document' ||
        request.destination === 'script' ||
        request.destination === 'style' ||
        url.pathname.startsWith('/static/')
    ) {
        event.respondWith(
            caches.match(request).then(cachedResponse => {
                if (cachedResponse) {
                    console.log('SA Assistant SW: Serving from cache:', url.pathname);
                    return cachedResponse;
                }

                // If not in cache, fetch and cache it
                return fetch(request)
                    .then(response => {
                        // Only cache successful responses
                        if (response.status === 200) {
                            const responseClone = response.clone();
                            caches.open(STATIC_CACHE_NAME).then(cache => {
                                cache.put(request, responseClone);
                            });
                        }
                        return response;
                    })
                    .catch(error => {
                        console.error('SA Assistant SW: Network request failed:', error);

                        // Return offline fallback for HTML requests
                        if (request.destination === 'document') {
                            return caches.match('/');
                        }

                        throw error;
                    });
            })
        );
        return;
    }

    // Handle API requests with network-first strategy
    if (
        url.pathname.startsWith('/api/') ||
        url.pathname === '/conversations' ||
        url.pathname.startsWith('/conversation/')
    ) {
        event.respondWith(
            fetch(request)
                .then(response => {
                    // Cache successful GET requests
                    if (request.method === 'GET' && response.status === 200) {
                        const responseClone = response.clone();
                        caches.open(DATA_CACHE_NAME).then(cache => {
                            cache.put(request, responseClone);
                        });
                    }
                    return response;
                })
                .catch(_error => {
                    console.log('SA Assistant SW: Network failed, trying cache:', url.pathname);

                    // Try to serve from cache if network fails
                    return caches.match(request).then(cachedResponse => {
                        if (cachedResponse) {
                            console.log('SA Assistant SW: Serving API from cache:', url.pathname);
                            return cachedResponse;
                        }

                        // Return error response for failed API calls
                        return new Response(
                            JSON.stringify({
                                error: 'Offline - Unable to fetch data',
                                message: 'Please check your internet connection and try again.'
                            }),
                            {
                                status: 503,
                                statusText: 'Service Unavailable',
                                headers: {
                                    'Content-Type': 'application/json'
                                }
                            }
                        );
                    });
                })
        );
        return;
    }

    // For all other requests, just pass through
    event.respondWith(fetch(request));
});

// Background sync for conversation updates
self.addEventListener('sync', event => {
    console.log('SA Assistant SW: Background sync triggered:', event.tag);

    if (event.tag === 'conversation-sync') {
        event.waitUntil(self.syncConversations());
    }
});

// Push notification handling
self.addEventListener('push', event => {
    console.log('SA Assistant SW: Push message received');

    let notificationData = {
        title: 'AI SA Assistant',
        body: 'Your query has been processed',
        data: {
            url: '/'
        }
    };

    if (event.data) {
        try {
            const data = event.data.json();
            notificationData = { ...notificationData, ...data };
        } catch (error) {
            console.error('SA Assistant SW: Failed to parse push data:', error);
        }
    }

    event.waitUntil(
        self.registration.showNotification(notificationData.title, {
            body: notificationData.body,
            data: notificationData.data,
            actions: [
                {
                    action: 'view',
                    title: 'View Response'
                },
                {
                    action: 'dismiss',
                    title: 'Dismiss'
                }
            ]
        })
    );
});

// Notification click handling
self.addEventListener('notificationclick', event => {
    console.log('SA Assistant SW: Notification clicked:', event.action);

    event.notification.close();

    if (event.action === 'view' || !event.action) {
        const url = event.notification.data?.url || '/';

        event.waitUntil(
            clients.matchAll({ type: 'window', includeUncontrolled: true }).then(clientList => {
                // Check if there's already a window open
                for (const client of clientList) {
                    if (client.url.includes(self.location.origin) && 'focus' in client) {
                        return client.focus();
                    }
                }

                // Open new window if none exists
                if (clients.openWindow) {
                    return clients.openWindow(url);
                }
                return Promise.resolve();
            })
        );
    }
});

// Helper function to sync conversations in background
self.syncConversations = async function () {
    try {
        console.log('SA Assistant SW: Syncing conversations...');

        const response = await fetch('/conversations');
        if (response.ok) {
            const conversations = await response.json();

            // Cache the updated conversations
            const cache = await caches.open(DATA_CACHE_NAME);
            await cache.put(
                '/conversations',
                new Response(JSON.stringify(conversations), {
                    headers: { 'Content-Type': 'application/json' }
                })
            );

            console.log('SA Assistant SW: Conversations synced successfully');
        }
    } catch (error) {
        console.error('SA Assistant SW: Failed to sync conversations:', error);
    }
};

// Message handling for communication with main thread
self.addEventListener('message', event => {
    console.log('SA Assistant SW: Message received:', event.data);

    if (event.data && event.data.type === 'SKIP_WAITING') {
        self.skipWaiting();
    }

    if (event.data && event.data.type === 'GET_VERSION') {
        event.ports[0].postMessage({ version: CACHE_NAME });
    }
});

console.log('SA Assistant SW: Service Worker loaded successfully');
