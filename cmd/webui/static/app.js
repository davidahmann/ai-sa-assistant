// AI SA Assistant - Frontend JavaScript

class ChatApp {
    constructor() {
        this.conversations = new Map();
        this.currentConversationId = null;
        this.isLoading = false;

        this.initializeElements();
        this.bindEvents();
        this.loadConversations();

        // Auto-resize textarea
        this.setupAutoResize();
    }

    initializeElements() {
        // Sidebar elements
        this.sidebar = document.getElementById('sidebar');
        this.sidebarOverlay = document.getElementById('sidebarOverlay');
        this.conversationsList = document.getElementById('conversationsList');
        this.newConversationBtn = document.getElementById('newConversationBtn');
        this.mobileMenuBtn = document.getElementById('mobileMenuBtn');

        // Chat elements
        this.messagesContainer = document.getElementById('messagesContainer');
        this.messageInput = document.getElementById('messageInput');
        this.sendBtn = document.getElementById('sendBtn');
        this.conversationTitle = document.getElementById('conversationTitle');
        this.clearChatBtn = document.getElementById('clearChatBtn');
        this.characterCount = document.getElementById('characterCount');

        // Loading and toast elements
        this.loadingIndicator = document.getElementById('loadingIndicator');
        this.toastContainer = document.getElementById('toastContainer');
    }

    bindEvents() {
        // Send message events
        this.sendBtn.addEventListener('click', () => this.sendMessage());
        this.messageInput.addEventListener('keydown', (e) => this.handleKeyDown(e));
        this.messageInput.addEventListener('input', () => this.updateSendButton());

        // Sidebar events
        this.newConversationBtn.addEventListener('click', () => this.createNewConversation());
        this.mobileMenuBtn.addEventListener('click', () => this.toggleSidebar());
        this.sidebarOverlay.addEventListener('click', () => this.closeSidebar());
        this.clearChatBtn.addEventListener('click', () => this.clearCurrentConversation());

        // Example question buttons
        document.addEventListener('click', (e) => {
            if (e.target.classList.contains('example-btn')) {
                const message = e.target.getAttribute('data-message');
                this.messageInput.value = message;
                this.updateSendButton();
                this.sendMessage();
            }
        });

        // Character count
        this.messageInput.addEventListener('input', () => this.updateCharacterCount());

        // Window resize
        window.addEventListener('resize', () => this.handleResize());
    }

    setupAutoResize() {
        this.messageInput.addEventListener('input', () => {
            this.messageInput.style.height = 'auto';
            this.messageInput.style.height = Math.min(this.messageInput.scrollHeight, 120) + 'px';
        });
    }

    async loadConversations() {
        try {
            const response = await fetch('/conversations');
            if (response.ok) {
                const conversations = await response.json();
                this.displayConversations(conversations);

                // Load the first conversation or create new one
                if (conversations.length > 0) {
                    this.loadConversation(conversations[0].id);
                } else {
                    this.createNewConversation();
                }
            }
        } catch (error) {
            console.error('Failed to load conversations:', error);
            this.showToast('Failed to load conversations', 'error');
        }
    }

    displayConversations(conversations) {
        this.conversationsList.innerHTML = '';

        conversations.forEach(conv => {
            this.conversations.set(conv.id, conv);

            const item = document.createElement('div');
            item.className = 'conversation-item';
            item.dataset.id = conv.id;

            const timeAgo = this.formatTimeAgo(new Date(conv.updated_at));

            item.innerHTML = `
                <div class="conversation-title" data-id="${conv.id}">
                    <span class="title-text">${this.escapeHtml(conv.title)}</span>
                    <input class="title-input" type="text" value="${this.escapeHtml(conv.title)}" style="display: none;">
                </div>
                <div class="conversation-meta">
                    <span>${conv.message_count} messages</span>
                    <span>${timeAgo}</span>
                    <div class="conversation-actions">
                        <button class="action-btn edit-btn" data-id="${conv.id}" title="Edit Title">
                            ✏️
                        </button>
                        <button class="action-btn delete-btn" data-id="${conv.id}" title="Delete">
                            🗑️
                        </button>
                    </div>
                </div>
            `;

            // Click to load conversation
            item.addEventListener('click', (e) => {
                if (!e.target.classList.contains('action-btn') && !e.target.classList.contains('title-input')) {
                    this.loadConversation(conv.id);
                    this.closeSidebar();
                }
            });

            // Edit conversation title
            const editBtn = item.querySelector('.edit-btn');
            editBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.startEditingTitle(conv.id);
            });

            // Delete conversation
            const deleteBtn = item.querySelector('.delete-btn');
            deleteBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.deleteConversation(conv.id);
            });

            this.conversationsList.appendChild(item);
        });
    }

    async loadConversation(conversationId) {
        try {
            const response = await fetch(`/conversations/${conversationId}`);
            if (response.ok) {
                const conversation = await response.json();
                this.currentConversationId = conversationId;
                this.conversations.set(conversationId, conversation);

                // Update UI
                this.conversationTitle.textContent = conversation.title;
                this.displayMessages(conversation.messages);
                this.setActiveConversation(conversationId);
            }
        } catch (error) {
            console.error('Failed to load conversation:', error);
            this.showToast('Failed to load conversation', 'error');
        }
    }

    displayMessages(messages) {
        this.messagesContainer.innerHTML = '';

        if (messages.length === 0) {
            this.showWelcomeMessage();
            return;
        }

        messages.forEach(message => {
            this.addMessageToUI(message);
        });

        this.scrollToBottom();
    }

    showWelcomeMessage() {
        this.messagesContainer.innerHTML = `
            <div class="welcome-message">
                <h2>Welcome to AI SA Assistant</h2>
                <p>Ask me about cloud architecture, migrations, security compliance, or disaster recovery planning.</p>
                <div class="example-questions">
                    <h4>Try asking:</h4>
                    <button class="example-btn" data-message="Generate a high-level lift-and-shift plan for migrating 120 on-prem Windows and Linux VMs to AWS">
                        AWS Migration Plan
                    </button>
                    <button class="example-btn" data-message="Design a DR solution in Azure for critical workloads with RTO = 2 hours and RPO = 15 minutes">
                        Azure DR Solution
                    </button>
                    <button class="example-btn" data-message="Outline a hybrid reference architecture connecting our on-prem VMware environment to Azure">
                        Azure Hybrid Architecture
                    </button>
                </div>
            </div>
        `;
    }

    addMessageToUI(message) {
        const messageEl = document.createElement('div');
        messageEl.className = `message ${message.role}`;

        const avatarText = message.role === 'user' ? 'SA' : 'AI';
        const timeStr = this.formatTime(new Date(message.timestamp));

        messageEl.innerHTML = `
            <div class="message-avatar">${avatarText}</div>
            <div class="message-content">
                <div class="message-bubble">
                    ${this.formatMessageContent(message.content)}
                </div>
                <div class="message-time">${timeStr}</div>
            </div>
        `;

        this.messagesContainer.appendChild(messageEl);
    }

    formatMessageContent(content) {
        // Enhanced text formatting with diagram and code block support
        let formattedContent = this.escapeHtml(content);

        // Process code blocks first (including mermaid diagrams)
        formattedContent = this.processCodeBlocks(formattedContent);

        // Convert remaining newlines to <br>
        formattedContent = formattedContent.replace(/\n/g, '<br>');

        return formattedContent;
    }

    processCodeBlocks(content) {
        // Regex to match code blocks: ```language\ncontent\n```
        const codeBlockRegex = /```(\w*)\n?([\s\S]*?)```/g;

        return content.replace(codeBlockRegex, (match, language, code) => {
            const trimmedCode = code.trim();
            const lang = language.toLowerCase();

            // Check if it's a mermaid diagram
            if (lang === 'mermaid' || this.isMermaidDiagram(trimmedCode)) {
                return this.renderMermaidDiagram(trimmedCode);
            }

            // Otherwise, render as syntax-highlighted code block
            return this.renderCodeBlock(trimmedCode, lang);
        });
    }

    isMermaidDiagram(code) {
        // Check if the code contains mermaid diagram syntax
        const mermaidKeywords = ['graph TD', 'graph LR', 'flowchart', 'sequenceDiagram', 'classDiagram', 'gitgraph'];
        return mermaidKeywords.some(keyword => code.includes(keyword));
    }

    renderMermaidDiagram(mermaidCode) {
        const diagramId = 'diagram-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);

        // Create container with loading state
        const container = `
            <div class="diagram-container" id="container-${diagramId}">
                <div class="diagram-header">
                    <span class="diagram-title">Architecture Diagram</span>
                    <div class="diagram-actions">
                        <button class="diagram-btn zoom-btn" title="Zoom In/Out" onclick="chatApp.toggleDiagramZoom('${diagramId}')">
                            🔍
                        </button>
                        <button class="diagram-btn copy-btn" title="Copy Diagram Code" onclick="chatApp.copyDiagramCode('${diagramId}')">
                            📋
                        </button>
                    </div>
                </div>
                <div class="diagram-content" id="${diagramId}">
                    <div class="diagram-loading">
                        <div class="spinner"></div>
                        <span>Rendering diagram...</span>
                    </div>
                </div>
                <div class="diagram-source" style="display: none;" data-source="${this.escapeHtml(mermaidCode)}"></div>
            </div>
        `;

        // Schedule diagram rendering after DOM update
        setTimeout(() => this.renderMermaidAsync(diagramId, mermaidCode), 100);

        return container;
    }

    async renderMermaidAsync(diagramId, mermaidCode) {
        try {
            // Initialize mermaid if not already done
            if (typeof mermaid !== 'undefined' && !window.mermaidInitialized) {
                mermaid.initialize({
                    startOnLoad: false,
                    theme: 'default',
                    securityLevel: 'strict',
                    er: { layoutDirection: 'TB' },
                    flowchart: { useMaxWidth: true, htmlLabels: true }
                });
                window.mermaidInitialized = true;
            }

            const element = document.getElementById(diagramId);
            if (!element) return;

            // Clear loading state
            element.innerHTML = '';

            // Render the diagram
            if (typeof mermaid !== 'undefined') {
                const { svg } = await mermaid.render(diagramId + '-svg', mermaidCode);
                element.innerHTML = svg;
                element.classList.add('diagram-rendered');
            } else {
                throw new Error('Mermaid library not loaded');
            }
        } catch (error) {
            console.error('Failed to render mermaid diagram:', error);
            this.renderDiagramFallback(diagramId, mermaidCode, error.message);
        }
    }

    renderDiagramFallback(diagramId, mermaidCode, errorMessage) {
        const element = document.getElementById(diagramId);
        if (!element) return;

        element.innerHTML = `
            <div class="diagram-error">
                <div class="error-icon">⚠️</div>
                <div class="error-text">
                    <strong>Diagram Rendering Failed</strong>
                    <br><small>${this.escapeHtml(errorMessage)}</small>
                </div>
            </div>
            <details class="diagram-fallback">
                <summary>View Diagram Code</summary>
                <pre><code>${this.escapeHtml(mermaidCode)}</code></pre>
            </details>
        `;
    }

    renderCodeBlock(code, language) {
        const codeId = 'code-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);
        const displayLang = language || 'text';

        const container = `
            <div class="code-container" id="container-${codeId}">
                <div class="code-header">
                    <span class="code-language">${displayLang}</span>
                    <div class="code-actions">
                        <button class="code-btn copy-btn" title="Copy Code" onclick="chatApp.copyCode('${codeId}')">
                            📋
                        </button>
                    </div>
                </div>
                <pre class="code-block"><code id="${codeId}" class="language-${language || 'text'}">${this.escapeHtml(code)}</code></pre>
            </div>
        `;

        // Schedule syntax highlighting after DOM update
        setTimeout(() => this.highlightCodeAsync(codeId), 100);

        return container;
    }

    async highlightCodeAsync(codeId) {
        try {
            const element = document.getElementById(codeId);
            if (!element) return;

            // Apply syntax highlighting if Prism is available
            if (typeof Prism !== 'undefined') {
                Prism.highlightElement(element);
            }
        } catch (error) {
            console.error('Failed to highlight code:', error);
        }
    }

    // Interactive feature methods
    toggleDiagramZoom(diagramId) {
        const container = document.getElementById('container-' + diagramId);
        if (container) {
            container.classList.toggle('diagram-zoomed');
        }
    }

    copyDiagramCode(diagramId) {
        const container = document.getElementById('container-' + diagramId);
        const sourceElement = container?.querySelector('.diagram-source');
        if (sourceElement) {
            const source = sourceElement.dataset.source;
            this.copyToClipboard(source, 'Diagram code copied to clipboard!');
        }
    }

    copyCode(codeId) {
        const element = document.getElementById(codeId);
        if (element) {
            const code = element.textContent;
            this.copyToClipboard(code, 'Code copied to clipboard!');
        }
    }

    copyToClipboard(text, successMessage) {
        if (navigator.clipboard && window.isSecureContext) {
            navigator.clipboard.writeText(text).then(() => {
                this.showToast(successMessage, 'success');
            }).catch(err => {
                console.error('Failed to copy to clipboard:', err);
                this.fallbackCopyToClipboard(text, successMessage);
            });
        } else {
            this.fallbackCopyToClipboard(text, successMessage);
        }
    }

    fallbackCopyToClipboard(text, successMessage) {
        const textArea = document.createElement('textarea');
        textArea.value = text;
        textArea.style.position = 'fixed';
        textArea.style.left = '-999999px';
        textArea.style.top = '-999999px';
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();

        try {
            document.execCommand('copy');
            this.showToast(successMessage, 'success');
        } catch (err) {
            console.error('Fallback copy failed:', err);
            this.showToast('Copy failed. Please select and copy manually.', 'error');
        } finally {
            document.body.removeChild(textArea);
        }
    }

    async sendMessage() {
        const message = this.messageInput.value.trim();
        if (!message || this.isLoading) return;

        this.isLoading = true;
        this.showLoading(true);
        this.messageInput.value = '';
        this.updateSendButton();
        this.updateCharacterCount();

        // Hide welcome message if present
        const welcomeMsg = this.messagesContainer.querySelector('.welcome-message');
        if (welcomeMsg) {
            welcomeMsg.remove();
        }

        // Add user message to UI immediately
        const userMessage = {
            id: Date.now().toString(),
            role: 'user',
            content: message,
            timestamp: new Date().toISOString()
        };
        this.addMessageToUI(userMessage);
        this.scrollToBottom();

        try {
            const response = await fetch('/chat', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    message: message,
                    conversation_id: this.currentConversationId
                })
            });

            if (response.ok) {
                const data = await response.json();
                if (data.error) {
                    this.showToast(data.error, 'error');
                } else {
                    // Update current conversation ID
                    this.currentConversationId = data.conversation_id;

                    // Add assistant message to UI
                    this.addMessageToUI(data.message);
                    this.scrollToBottom();

                    // Reload conversations to update sidebar
                    this.loadConversations();
                }
            } else {
                this.showToast('Failed to send message', 'error');
            }
        } catch (error) {
            console.error('Failed to send message:', error);
            this.showToast('Failed to send message', 'error');
        } finally {
            this.isLoading = false;
            this.showLoading(false);
        }
    }

    async createNewConversation() {
        try {
            const response = await fetch('/conversations', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                }
            });

            if (response.ok) {
                const conversation = await response.json();
                this.currentConversationId = conversation.id;
                this.conversations.set(conversation.id, conversation);

                // Update UI
                this.conversationTitle.textContent = conversation.title;
                this.showWelcomeMessage();
                this.loadConversations();
                this.closeSidebar();
            }
        } catch (error) {
            console.error('Failed to create conversation:', error);
            this.showToast('Failed to create conversation', 'error');
        }
    }

    async deleteConversation(conversationId) {
        if (!confirm('Are you sure you want to delete this conversation?')) return;

        try {
            const response = await fetch(`/conversations/${conversationId}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                this.conversations.delete(conversationId);

                // If this was the current conversation, create a new one
                if (this.currentConversationId === conversationId) {
                    this.createNewConversation();
                } else {
                    this.loadConversations();
                }

                this.showToast('Conversation deleted', 'success');
            }
        } catch (error) {
            console.error('Failed to delete conversation:', error);
            this.showToast('Failed to delete conversation', 'error');
        }
    }

    clearCurrentConversation() {
        if (!confirm('Clear this conversation? This cannot be undone.')) return;

        if (this.currentConversationId) {
            this.deleteConversation(this.currentConversationId);
        }
    }

    setActiveConversation(conversationId) {
        // Remove active class from all items
        document.querySelectorAll('.conversation-item').forEach(item => {
            item.classList.remove('active');
        });

        // Add active class to current item
        const activeItem = document.querySelector(`[data-id="${conversationId}"]`);
        if (activeItem) {
            activeItem.classList.add('active');
        }
    }

    startEditingTitle(conversationId) {
        const titleContainer = document.querySelector(`.conversation-title[data-id="${conversationId}"]`);
        if (!titleContainer) return;

        const titleText = titleContainer.querySelector('.title-text');
        const titleInput = titleContainer.querySelector('.title-input');

        if (!titleText || !titleInput) return;

        // Hide text, show input
        titleText.style.display = 'none';
        titleInput.style.display = 'block';
        titleInput.focus();
        titleInput.select();

        // Handle input events
        const finishEditing = () => this.finishEditingTitle(conversationId);
        const cancelEditing = () => this.cancelEditingTitle(conversationId);

        titleInput.addEventListener('blur', finishEditing, { once: true });
        titleInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                finishEditing();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                cancelEditing();
            }
        }, { once: true });
    }

    async finishEditingTitle(conversationId) {
        const titleContainer = document.querySelector(`.conversation-title[data-id="${conversationId}"]`);
        if (!titleContainer) return;

        const titleText = titleContainer.querySelector('.title-text');
        const titleInput = titleContainer.querySelector('.title-input');

        if (!titleText || !titleInput) return;

        const newTitle = titleInput.value.trim();
        const conversation = this.conversations.get(conversationId);

        if (!newTitle || !conversation) {
            this.cancelEditingTitle(conversationId);
            return;
        }

        // Don't update if title hasn't changed
        if (newTitle === conversation.title) {
            this.cancelEditingTitle(conversationId);
            return;
        }

        try {
            const response = await fetch(`/conversations/${conversationId}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    title: newTitle
                })
            });

            if (response.ok) {
                // Update local data
                conversation.title = newTitle;
                this.conversations.set(conversationId, conversation);

                // Update UI
                titleText.textContent = newTitle;
                titleText.style.display = 'block';
                titleInput.style.display = 'none';

                // Update main conversation title if this is the current conversation
                if (this.currentConversationId === conversationId) {
                    this.conversationTitle.textContent = newTitle;
                }

                this.showToast('Conversation title updated', 'success');
            } else {
                this.showToast('Failed to update conversation title', 'error');
                this.cancelEditingTitle(conversationId);
            }
        } catch (error) {
            console.error('Failed to update conversation title:', error);
            this.showToast('Failed to update conversation title', 'error');
            this.cancelEditingTitle(conversationId);
        }
    }

    cancelEditingTitle(conversationId) {
        const titleContainer = document.querySelector(`.conversation-title[data-id="${conversationId}"]`);
        if (!titleContainer) return;

        const titleText = titleContainer.querySelector('.title-text');
        const titleInput = titleContainer.querySelector('.title-input');

        if (!titleText || !titleInput) return;

        const conversation = this.conversations.get(conversationId);
        if (conversation) {
            titleInput.value = conversation.title;
        }

        // Show text, hide input
        titleText.style.display = 'block';
        titleInput.style.display = 'none';
    }

    updateSendButton() {
        const hasText = this.messageInput.value.trim().length > 0;
        this.sendBtn.disabled = !hasText || this.isLoading;
    }

    updateCharacterCount() {
        const count = this.messageInput.value.length;
        this.characterCount.textContent = `${count}/2000`;

        if (count > 1800) {
            this.characterCount.style.color = '#ef4444';
        } else if (count > 1500) {
            this.characterCount.style.color = '#f59e0b';
        } else {
            this.characterCount.style.color = '#64748b';
        }
    }

    handleKeyDown(e) {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            this.sendMessage();
        }
    }

    toggleSidebar() {
        this.sidebar.classList.toggle('open');
        this.sidebarOverlay.classList.toggle('show');
    }

    closeSidebar() {
        this.sidebar.classList.remove('open');
        this.sidebarOverlay.classList.remove('show');
    }

    handleResize() {
        if (window.innerWidth > 768) {
            this.closeSidebar();
        }
    }

    showLoading(show) {
        if (show) {
            this.loadingIndicator.classList.add('show');
        } else {
            this.loadingIndicator.classList.remove('show');
        }
    }

    showToast(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.textContent = message;

        this.toastContainer.appendChild(toast);

        // Auto remove after 3 seconds
        setTimeout(() => {
            toast.remove();
        }, 3000);
    }

    scrollToBottom() {
        requestAnimationFrame(() => {
            this.messagesContainer.scrollTop = this.messagesContainer.scrollHeight;
        });
    }

    formatTime(date) {
        return date.toLocaleTimeString('en-US', {
            hour: '2-digit',
            minute: '2-digit'
        });
    }

    formatTimeAgo(date) {
        const now = new Date();
        const diff = now - date;
        const minutes = Math.floor(diff / 60000);
        const hours = Math.floor(diff / 3600000);
        const days = Math.floor(diff / 86400000);

        if (minutes < 1) return 'just now';
        if (minutes < 60) return `${minutes}m ago`;
        if (hours < 24) return `${hours}h ago`;
        if (days < 7) return `${days}d ago`;

        return date.toLocaleDateString();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Initialize the app when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.chatApp = new ChatApp();
});
