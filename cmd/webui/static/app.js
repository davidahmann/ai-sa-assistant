// AI SA Assistant - Frontend JavaScript

class ChatApp {
    constructor() {
        this.conversations = new Map();
        this.currentConversationId = null;
        this.isLoading = false;

        // Streaming support
        this.isStreaming = false;
        this.currentEventSource = null;
        this.streamingProgressElement = null;
        this.streamingResponseElement = null;

        this.initializeElements();
        this.bindEvents();
        this.loadConversations();

        // Auto-resize textarea
        this.setupAutoResize();

        // Mobile touch gestures
        this.setupTouchGestures();

        // Initialize theme
        this.initializeTheme();
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

        // Theme toggle element
        this.themeToggle = document.getElementById('themeToggle');
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

        // Theme toggle
        this.themeToggle.addEventListener('click', () => this.toggleTheme());
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

        const messageContentHTML = this.formatMessageContent(message.content);
        const sourcesHTML = this.formatMessageSources(message);
        const contextHTML = this.formatMessageContext(message);

        messageEl.innerHTML = `
            <div class="message-avatar">${avatarText}</div>
            <div class="message-content">
                <div class="message-bubble">
                    ${messageContentHTML}
                </div>
                ${sourcesHTML}
                ${contextHTML}
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

    formatMessageSources(message) {
        // Only show sources for assistant messages that have metadata
        if (message.role !== 'assistant' || !message.metadata) {
            return '';
        }

        const metadata = message.metadata;
        const contextSources = metadata.context_sources || [];
        const webSources = metadata.web_sources || [];
        const processingStats = metadata.processing_stats || {};

        if (contextSources.length === 0 && webSources.length === 0) {
            return '';
        }

        const totalSources = contextSources.length + webSources.length;
        const sourceId = `sources-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

        return `
            <div class="message-sources">
                <div class="sources-header" onclick="chatApp.toggleSources('${sourceId}')">
                    <span class="sources-icon">📋</span>
                    <span class="sources-title">Context Sources Used (${totalSources} total)</span>
                    <span class="sources-toggle">▼</span>
                </div>
                <div class="sources-content" id="${sourceId}" style="display: none;">
                    ${this.formatInternalSources(contextSources)}
                    ${this.formatWebSources(webSources)}
                    ${this.formatLLMSynthesis(processingStats)}
                </div>
            </div>
        `;
    }

    formatMessageContext(message) {
        // Only show context for assistant messages that have pipeline decisions
        if (message.role !== 'assistant' || !message.metadata || !message.metadata.pipeline_decision) {
            return '';
        }

        const pipeline = message.metadata.pipeline_decision;
        const processingStats = message.metadata.processing_stats || {};
        const contextId = `context-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

        return `
            <div class="message-context">
                <div class="context-header" onclick="chatApp.toggleContext('${contextId}')">
                    <span class="context-icon">🔍</span>
                    <span class="context-title">Pipeline Decisions & Performance</span>
                    <span class="context-toggle">▼</span>
                </div>
                <div class="context-content" id="${contextId}" style="display: none;">
                    ${this.formatPipelineDecisions(pipeline)}
                    ${this.formatProcessingStats(processingStats)}
                    ${this.formatTrustIndicators(message.metadata)}
                </div>
            </div>
        `;
    }

    formatInternalSources(sources) {
        if (!sources || sources.length === 0) return '';

        let html = `
            <div class="internal-sources">
                <h5>📄 Internal Documents (${sources.length} chunks)</h5>
                <ul class="source-list">
        `;

        sources.forEach(source => {
            const usedIcon = source.used ? '✓' : '○';
            const usedClass = source.used ? 'used' : 'unused';
            const confidence = source.confidence ? `(confidence: ${source.confidence.toFixed(2)})` : '';

            html += `
                <li class="source-item ${usedClass}">
                    <div class="source-info">
                        <span class="source-icon">${usedIcon}</span>
                        <strong>${this.escapeHtml(source.title || source.source_id)}</strong>
                        <span class="confidence">${confidence}</span>
                    </div>
                    <div class="source-preview">${this.escapeHtml(source.preview || '')}</div>
                    <div class="source-meta">${source.token_count || 0} tokens • ${source.source_type || 'internal_doc'}</div>
                </li>
            `;
        });

        html += `
                </ul>
            </div>
        `;

        return html;
    }

    formatWebSources(sources) {
        if (!sources || sources.length === 0) return '';

        let html = `
            <div class="web-sources">
                <h5>🌐 Web Search Results (${sources.length} articles)</h5>
                <ul class="source-list">
        `;

        sources.forEach(source => {
            const usedIcon = source.used ? '✓' : '○';
            const usedClass = source.used ? 'used' : 'unused';
            const confidence = source.confidence ? `(confidence: ${source.confidence.toFixed(2)})` : '';

            html += `
                <li class="source-item ${usedClass}">
                    <div class="source-info">
                        <span class="source-icon">${usedIcon}</span>
                        <a href="${this.escapeHtml(source.url)}" target="_blank" rel="noopener noreferrer">
                            ${this.escapeHtml(source.title || source.url)}
                        </a>
                        <span class="confidence">${confidence}</span>
                    </div>
                    <div class="source-preview">${this.escapeHtml(source.snippet || '')}</div>
                    <div class="source-meta">${source.domain || ''} • ${source.freshness || 'recent'}</div>
                </li>
            `;
        });

        html += `
                </ul>
            </div>
        `;

        return html;
    }

    formatLLMSynthesis(stats) {
        if (!stats || !stats.model_used) return '';

        return `
            <div class="llm-synthesis">
                <h5>🤖 LLM Synthesis</h5>
                <div class="synthesis-info">
                    <div class="model-info">Model: ${this.escapeHtml(stats.model_used)}</div>
                    <div class="token-info">Tokens: ${stats.input_tokens || 0} input / ${stats.output_tokens || 0} output</div>
                    <div class="temp-info">Temperature: ${stats.temperature || 0}</div>
                </div>
            </div>
        `;
    }

    formatPipelineDecisions(pipeline) {
        if (!pipeline) return '';

        let html = `
            <div class="pipeline-decisions">
                <h5>🔍 Pipeline Decisions</h5>
                <div class="pipeline-info">
                    <div class="query-type">Query Type: <span class="highlight">${this.escapeHtml(pipeline.query_type || 'Unknown')}</span></div>
        `;

        if (pipeline.metadata_filters_applied && pipeline.metadata_filters_applied.length > 0) {
            html += `<div class="filters">Metadata Filters: ${pipeline.metadata_filters_applied.join(', ')}</div>`;
        }

        if (pipeline.fallback_search_used) {
            html += `<div class="fallback">🔄 Fallback search used due to insufficient initial results</div>`;
        }

        if (pipeline.web_search_triggered) {
            html += `<div class="web-search">🌐 Web search triggered for fresh information</div>`;
            if (pipeline.freshness_keywords && pipeline.freshness_keywords.length > 0) {
                html += `<div class="freshness">Freshness keywords: ${pipeline.freshness_keywords.join(', ')}</div>`;
            }
        }

        html += `
                    <div class="context-stats">Context: ${pipeline.context_items_filtered || 0} items filtered → ${pipeline.context_items_used || 0} used</div>
        `;

        if (pipeline.reasoning) {
            html += `<div class="reasoning">Reasoning: ${this.escapeHtml(pipeline.reasoning)}</div>`;
        }

        html += `
                </div>
            </div>
        `;

        return html;
    }

    formatProcessingStats(stats) {
        if (!stats || !stats.total_processing_time_ms) return '';

        const totalTime = this.formatDuration(stats.total_processing_time_ms);
        const cost = stats.estimated_cost_usd ? `$${stats.estimated_cost_usd.toFixed(4)}` : '';

        return `
            <div class="processing-stats">
                <h5>⏱️ Processing Statistics</h5>
                <div class="stats-info">
                    <div class="time-info">Total Time: ${totalTime}</div>
                    <div class="token-info">Total Tokens: ${stats.total_tokens || 0}</div>
                    ${cost ? `<div class="cost-info">Estimated Cost: ${cost}</div>` : ''}
                    <div class="performance-bar">${this.createPerformanceBar(stats)}</div>
                </div>
            </div>
        `;
    }

    formatTrustIndicators(metadata) {
        if (!metadata) return '';

        // Calculate trust indicators on the client side
        const contextSources = metadata.context_sources || [];
        const webSources = metadata.web_sources || [];
        const pipeline = metadata.pipeline_decision || {};

        let sourceQuality = 0;
        let totalSources = 0;

        contextSources.forEach(source => {
            if (source.confidence) {
                sourceQuality += source.confidence;
                totalSources++;
            }
        });

        webSources.forEach(source => {
            if (source.confidence) {
                sourceQuality += source.confidence;
                totalSources++;
            }
        });

        if (totalSources === 0) return '';

        const avgQuality = sourceQuality / totalSources;
        const confidenceLevel = avgQuality >= 0.8 ? 'High' : avgQuality >= 0.6 ? 'Medium' : 'Low';
        const freshness = pipeline.web_search_triggered ? 'Recent' : 'Standard';

        const badges = [];
        if (avgQuality >= 0.8) badges.push('High Quality Sources');
        if (contextSources.length > 0) badges.push('Internal Documentation');
        if (pipeline.web_search_triggered) badges.push('Fresh Information');

        return `
            <div class="trust-indicators">
                <h5>🛡️ Trust Indicators</h5>
                <div class="trust-info">
                    <div class="overall-score">Overall Score: ${avgQuality.toFixed(2)}</div>
                    <div class="confidence-level">Confidence: ${confidenceLevel}</div>
                    <div class="freshness">Freshness: ${freshness}</div>
                    ${badges.length > 0 ? `<div class="trust-badges">${badges.map(badge => `<span class="badge">${badge}</span>`).join('')}</div>` : ''}
                </div>
            </div>
        `;
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
        if (!message || this.isLoading || this.isStreaming) return;

        // Check if streaming mode is enabled (we'll assume streaming by default for demo)
        const useStreaming = true; // This could be controlled by a UI toggle

        if (useStreaming) {
            return this.sendMessageStreaming(message);
        } else {
            return this.sendMessageTraditional(message);
        }
    }

    async sendMessageTraditional(message) {
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

    async sendMessageStreaming(message) {
        if (this.isStreaming) return; // Prevent multiple concurrent streams

        this.isStreaming = true;
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

        // Add streaming progress placeholder
        this.addStreamingProgressPlaceholder();

        try {
            // Initiate streaming request
            const response = await fetch('/chat/stream', {
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
                    this.removeStreamingProgress();
                } else {
                    // Update current conversation ID
                    this.currentConversationId = data.conversation_id;

                    // Start SSE stream
                    this.startEventStream(data.stream_id);
                }
            } else {
                this.showToast('Failed to send message', 'error');
                this.removeStreamingProgress();
            }
        } catch (error) {
            console.error('Failed to send message:', error);
            this.showToast('Failed to send message', 'error');
            this.removeStreamingProgress();
        } finally {
            // Don't reset isStreaming here - it will be reset when stream completes
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
        this.sendBtn.disabled = !hasText || this.isLoading || this.isStreaming;

        // Update button text based on state
        if (this.isStreaming) {
            this.sendBtn.textContent = 'Processing...';
        } else if (this.isLoading) {
            this.sendBtn.textContent = 'Sending...';
        } else {
            this.sendBtn.textContent = 'Send';
        }
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

    setupTouchGestures() {
        let touchStartX = 0;
        let touchStartY = 0;
        let touchEndX = 0;
        let touchEndY = 0;
        let isSwiping = false;
        let isHorizontalSwipe = false;

        const minSwipeDistance = 50;
        const maxVerticalDeviation = 100;
        const swipeThreshold = 0.3; // 30% of screen width

        // Touch start handler
        const handleTouchStart = (e) => {
            if (window.innerWidth > 768) return; // Only on mobile

            touchStartX = e.touches[0].clientX;
            touchStartY = e.touches[0].clientY;
            isSwiping = true;
            isHorizontalSwipe = false;
        };

        // Touch move handler
        const handleTouchMove = (e) => {
            if (!isSwiping || window.innerWidth > 768) return;

            touchEndX = e.touches[0].clientX;
            touchEndY = e.touches[0].clientY;

            const deltaX = Math.abs(touchEndX - touchStartX);
            const deltaY = Math.abs(touchEndY - touchStartY);

            // Determine if this is a horizontal swipe
            if (!isHorizontalSwipe && deltaX > 10) {
                isHorizontalSwipe = deltaX > deltaY;

                if (isHorizontalSwipe) {
                    // Prevent vertical scrolling during horizontal swipe
                    e.preventDefault();
                }
            }

            // Show visual feedback for swipe to open sidebar
            if (isHorizontalSwipe && touchStartX < 50 && (touchEndX - touchStartX) > 20) {
                const progress = Math.min((touchEndX - touchStartX) / (window.innerWidth * swipeThreshold), 1);
                this.showSwipeProgress(progress);
            }
        };

        // Touch end handler
        const handleTouchEnd = (e) => {
            if (!isSwiping || window.innerWidth > 768) return;

            const deltaX = touchEndX - touchStartX;
            const deltaY = Math.abs(touchEndY - touchStartY);

            // Only process horizontal swipes
            if (isHorizontalSwipe && Math.abs(deltaX) > minSwipeDistance && deltaY < maxVerticalDeviation) {
                // Swipe right from left edge - open sidebar
                if (deltaX > 0 && touchStartX < 50) {
                    this.openSidebar();
                }
                // Swipe left - close sidebar
                else if (deltaX < 0 && this.sidebar.classList.contains('open')) {
                    this.closeSidebar();
                }
            }

            // Reset state
            this.hideSwipeProgress();
            isSwiping = false;
            isHorizontalSwipe = false;
            touchStartX = 0;
            touchStartY = 0;
            touchEndX = 0;
            touchEndY = 0;
        };

        // Attach touch listeners to main content area
        this.messagesContainer.addEventListener('touchstart', handleTouchStart, { passive: false });
        this.messagesContainer.addEventListener('touchmove', handleTouchMove, { passive: false });
        this.messagesContainer.addEventListener('touchend', handleTouchEnd, { passive: true });

        // Also attach to input area for swipe gestures
        const inputContainer = document.querySelector('.input-container');
        if (inputContainer) {
            inputContainer.addEventListener('touchstart', handleTouchStart, { passive: false });
            inputContainer.addEventListener('touchmove', handleTouchMove, { passive: false });
            inputContainer.addEventListener('touchend', handleTouchEnd, { passive: true });
        }

        console.log('SA Assistant: Touch gestures initialized');
    }

    showSwipeProgress(progress) {
        // Add visual feedback during swipe
        if (!this.swipeIndicator) {
            this.swipeIndicator = document.createElement('div');
            this.swipeIndicator.className = 'swipe-indicator';
            document.body.appendChild(this.swipeIndicator);
        }

        this.swipeIndicator.style.width = `${progress * 100}%`;
        this.swipeIndicator.style.opacity = Math.min(progress * 2, 0.3);
    }

    hideSwipeProgress() {
        if (this.swipeIndicator) {
            this.swipeIndicator.style.width = '0%';
            this.swipeIndicator.style.opacity = '0';
        }
    }

    openSidebar() {
        this.sidebar.classList.add('open');
        this.sidebarOverlay.classList.add('show');

        // Add haptic feedback if available
        if (navigator.vibrate) {
            navigator.vibrate(10);
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

    // Source and context visibility toggle functions
    toggleSources(sourceId) {
        const element = document.getElementById(sourceId);
        const toggle = document.querySelector(`[onclick="chatApp.toggleSources('${sourceId}')"] .sources-toggle`);

        if (element.style.display === 'none') {
            element.style.display = 'block';
            toggle.textContent = '▲';
        } else {
            element.style.display = 'none';
            toggle.textContent = '▼';
        }
    }

    toggleContext(contextId) {
        const element = document.getElementById(contextId);
        const toggle = document.querySelector(`[onclick="chatApp.toggleContext('${contextId}')"] .context-toggle`);

        if (element.style.display === 'none') {
            element.style.display = 'block';
            toggle.textContent = '▲';
        } else {
            element.style.display = 'none';
            toggle.textContent = '▼';
        }
    }

    // Utility functions for formatting
    formatDuration(milliseconds) {
        if (milliseconds < 1000) {
            return `${milliseconds}ms`;
        }
        return `${(milliseconds / 1000).toFixed(1)}s`;
    }

    createPerformanceBar(stats) {
        const total = stats.total_processing_time_ms || 1;
        const retrieval = ((stats.retrieval_time_ms || 0) / total * 100).toFixed(0);
        const websearch = ((stats.web_search_time_ms || 0) / total * 100).toFixed(0);
        const synthesis = ((stats.synthesis_time_ms || 0) / total * 100).toFixed(0);

        return `Retrieval: ${retrieval}% | Web Search: ${websearch}% | Synthesis: ${synthesis}%`;
    }

    // Streaming Methods

    addStreamingProgressPlaceholder() {
        const progressElement = document.createElement('div');
        progressElement.className = 'message assistant streaming';
        progressElement.id = 'streaming-progress';

        progressElement.innerHTML = `
            <div class="message-avatar">AI</div>
            <div class="message-content">
                <div class="message-bubble">
                    <div class="streaming-progress">
                        <div class="progress-header">
                            <span class="progress-icon">⚡</span>
                            <span class="progress-title">Processing your request...</span>
                        </div>
                        <div class="progress-bar-container">
                            <div class="progress-bar" id="streaming-progress-bar">
                                <div class="progress-fill" style="width: 0%"></div>
                            </div>
                            <span class="progress-percentage">0%</span>
                        </div>
                        <div class="progress-details">
                            <div class="current-stage" id="current-stage">🔍 Initializing...</div>
                            <div class="stage-list" id="stage-list">
                                <div class="stage-item" data-stage="query_analysis">Query Analysis</div>
                                <div class="stage-item" data-stage="metadata_filter">Metadata Search</div>
                                <div class="stage-item" data-stage="embeddings">Embeddings</div>
                                <div class="stage-item" data-stage="vector_search">Vector Search</div>
                                <div class="stage-item" data-stage="freshness_detection">Freshness Check</div>
                                <div class="stage-item" data-stage="web_search">Web Search</div>
                                <div class="stage-item" data-stage="synthesis">LLM Synthesis</div>
                                <div class="stage-item" data-stage="diagram_rendering">Diagram Rendering</div>
                            </div>
                        </div>
                        <div class="stream-stats" id="stream-stats">
                            <span class="stat-item">⏱️ <span id="elapsed-time">0.0s</span></span>
                            <span class="stat-item">📊 <span id="processed-items">0</span> items</span>
                        </div>
                    </div>
                </div>
                <div class="message-time">${this.formatTime(new Date())}</div>
            </div>
        `;

        this.messagesContainer.appendChild(progressElement);
        this.streamingProgressElement = progressElement;
        this.scrollToBottom();

        // Start timer
        this.streamStartTime = Date.now();
        this.updateStreamTimer();
    }

    removeStreamingProgress() {
        if (this.streamingProgressElement) {
            this.streamingProgressElement.remove();
            this.streamingProgressElement = null;
        }

        if (this.streamTimerInterval) {
            clearInterval(this.streamTimerInterval);
            this.streamTimerInterval = null;
        }

        this.isStreaming = false;
        this.updateSendButton();
    }

    updateStreamTimer() {
        this.streamTimerInterval = setInterval(() => {
            if (this.streamStartTime) {
                const elapsed = (Date.now() - this.streamStartTime) / 1000;
                const elapsedTimeElement = document.getElementById('elapsed-time');
                if (elapsedTimeElement) {
                    elapsedTimeElement.textContent = elapsed.toFixed(1) + 's';
                }
            }
        }, 100);
    }

    updateStreamingProgress(progress, stage, message, data = {}) {
        if (!this.streamingProgressElement) return;

        // Update progress bar
        const progressFill = this.streamingProgressElement.querySelector('.progress-fill');
        const progressPercentage = this.streamingProgressElement.querySelector('.progress-percentage');
        if (progressFill && progressPercentage) {
            progressFill.style.width = progress + '%';
            progressPercentage.textContent = progress + '%';
        }

        // Update current stage
        const currentStageElement = document.getElementById('current-stage');
        if (currentStageElement) {
            currentStageElement.textContent = message;
        }

        // Highlight current stage in list
        const stageItems = this.streamingProgressElement.querySelectorAll('.stage-item');
        stageItems.forEach(item => {
            item.classList.remove('active', 'completed');
            const itemStage = item.getAttribute('data-stage');
            if (itemStage === stage) {
                item.classList.add('active');
            } else if (this.isStageCompleted(itemStage, stage)) {
                item.classList.add('completed');
            }
        });

        // Update stats if provided
        if (data.chunks_found !== undefined || data.results_found !== undefined) {
            const processedItemsElement = document.getElementById('processed-items');
            if (processedItemsElement) {
                const items = data.chunks_found || data.results_found || 0;
                processedItemsElement.textContent = items;
            }
        }

        this.scrollToBottom();
    }

    isStageCompleted(checkStage, currentStage) {
        const stageOrder = [
            'query_analysis',
            'metadata_filter',
            'embeddings',
            'vector_search',
            'freshness_detection',
            'web_search',
            'synthesis',
            'diagram_rendering'
        ];

        const checkIndex = stageOrder.indexOf(checkStage);
        const currentIndex = stageOrder.indexOf(currentStage);

        return checkIndex !== -1 && currentIndex !== -1 && checkIndex < currentIndex;
    }

    startEventStream(streamId) {
        if (this.currentEventSource) {
            this.currentEventSource.close();
        }

        const eventSourceUrl = `/stream/${streamId}`;
        this.currentEventSource = new EventSource(eventSourceUrl);

        this.currentEventSource.onopen = () => {
            console.log('SSE connection opened');
        };

        this.currentEventSource.onmessage = (event) => {
            try {
                const eventData = JSON.parse(event.data);
                this.handleStreamEvent(eventData);
            } catch (error) {
                console.error('Failed to parse stream event:', error);
            }
        };

        this.currentEventSource.onerror = (error) => {
            console.error('SSE connection error:', error);

            if (this.currentEventSource.readyState === EventSource.CLOSED) {
                console.log('SSE connection closed');
                this.handleStreamComplete();
            } else {
                // Connection failed
                this.handleStreamError('Connection to server lost');
            }
        };

        // Fallback timeout
        setTimeout(() => {
            if (this.isStreaming && this.currentEventSource) {
                this.handleStreamError('Stream timeout after 60 seconds');
            }
        }, 60000);
    }

    handleStreamEvent(eventData) {
        const { type, stage, message, progress, data } = eventData;

        console.log('Stream event:', eventData);

        switch (type) {
            case 'progress':
                this.updateStreamingProgress(progress, stage, message, data || {});
                break;

            case 'error':
                this.handleStreamError(message);
                break;

            case 'complete':
                this.handleStreamComplete(data);
                break;

            case 'metrics':
                // Handle performance metrics if needed
                break;

            default:
                console.warn('Unknown stream event type:', type);
        }
    }

    handleStreamComplete(data = {}) {
        if (this.currentEventSource) {
            this.currentEventSource.close();
            this.currentEventSource = null;
        }

        // Remove progress indicator
        this.removeStreamingProgress();

        // Add final response if provided
        if (data && data.response) {
            const assistantMessage = {
                id: Date.now().toString(),
                role: 'assistant',
                content: data.response.main_text,
                timestamp: new Date().toISOString(),
                metadata: {
                    has_diagram: !!data.response.diagram_code,
                    has_code: data.response.code_snippets && data.response.code_snippets.length > 0,
                    execution_time_ms: data.execution_time_ms,
                    services_used: data.services_used,
                    fallback_used: data.fallback_used,
                    diagram_url: data.response.diagram_url
                }
            };

            this.addMessageToUI(assistantMessage);
            this.scrollToBottom();

            // Reload conversations to update sidebar
            this.loadConversations();
        }

        this.isStreaming = false;
        this.updateSendButton();
    }

    handleStreamError(errorMessage) {
        console.error('Stream error:', errorMessage);

        if (this.currentEventSource) {
            this.currentEventSource.close();
            this.currentEventSource = null;
        }

        // Remove progress indicator
        this.removeStreamingProgress();

        // Show error message
        this.showToast(`Stream error: ${errorMessage}`, 'error');

        // Add error message to chat
        const errorMessageObj = {
            id: Date.now().toString(),
            role: 'assistant',
            content: `❌ Sorry, I encountered an error while processing your request: ${errorMessage}`,
            timestamp: new Date().toISOString()
        };

        this.addMessageToUI(errorMessageObj);
        this.scrollToBottom();

        this.isStreaming = false;
        this.updateSendButton();
    }

    // Theme Management Methods

    initializeTheme() {
        // Check for saved theme preference
        const savedTheme = localStorage.getItem('ai-sa-assistant-theme');

        // Check system preference
        const systemPrefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;

        // Determine initial theme
        let initialTheme = 'light';
        if (savedTheme) {
            initialTheme = savedTheme;
        } else if (systemPrefersDark) {
            initialTheme = 'dark';
        }

        // Apply theme
        this.setTheme(initialTheme);

        // Listen for system theme changes
        if (window.matchMedia) {
            const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
            mediaQuery.addEventListener('change', (e) => {
                // Only auto-switch if user hasn't manually set a preference
                if (!localStorage.getItem('ai-sa-assistant-theme')) {
                    this.setTheme(e.matches ? 'dark' : 'light');
                }
            });
        }
    }

    toggleTheme() {
        const currentTheme = document.documentElement.getAttribute('data-theme') || 'light';
        const newTheme = currentTheme === 'light' ? 'dark' : 'light';
        this.setTheme(newTheme);

        // Save user preference
        localStorage.setItem('ai-sa-assistant-theme', newTheme);

        // Show toast notification
        this.showToast(`Switched to ${newTheme} mode`, 'success');
    }

    setTheme(theme) {
        document.documentElement.setAttribute('data-theme', theme);

        // Update meta theme-color for mobile browsers
        const metaThemeColor = document.querySelector('meta[name="theme-color"]');
        if (metaThemeColor) {
            if (theme === 'dark') {
                metaThemeColor.setAttribute('content', '#1e293b');
            } else {
                metaThemeColor.setAttribute('content', '#2563eb');
            }
        }

        // Update Apple touch icon style
        const appleStatusBar = document.querySelector('meta[name="apple-mobile-web-app-status-bar-style"]');
        if (appleStatusBar) {
            appleStatusBar.setAttribute('content', theme === 'dark' ? 'black-translucent' : 'default');
        }
    }

    getCurrentTheme() {
        return document.documentElement.getAttribute('data-theme') || 'light';
    }
}

// Initialize the app when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.chatApp = new ChatApp();
});
