import React, { useState, useRef, useEffect, KeyboardEvent } from 'react';
import styles from './ArchitectChat.module.css';

interface Message {
  role: 'user' | 'assistant';
  content: string;
}

function renderMarkdown(text: string): string {
  // Code blocks (``` ... ```) — must come before inline code
  text = text.replace(/```[\w]*\n?([\s\S]*?)```/g, '<pre><code>$1</code></pre>');

  // Inline code
  text = text.replace(/`([^`]+)`/g, '<code>$1</code>');

  // Bold
  text = text.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');

  // Italic
  text = text.replace(/\*([^*]+)\*/g, '<em>$1</em>');

  // Bullet lists (lines starting with "- ")
  // Wrap consecutive bullet items in <ul>
  const lines = text.split('\n');
  const processed: string[] = [];
  let inList = false;

  for (const line of lines) {
    const bulletMatch = line.match(/^- (.+)$/);
    if (bulletMatch) {
      if (!inList) {
        processed.push('<ul>');
        inList = true;
      }
      processed.push(`<li>${bulletMatch[1]}</li>`);
    } else {
      if (inList) {
        processed.push('</ul>');
        inList = false;
      }
      processed.push(line);
    }
  }
  if (inList) processed.push('</ul>');

  return processed.join('\n');
}

export function ArchitectChat() {
  const [open, setOpen] = useState(false);
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (messagesEndRef.current) {
      messagesEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, loading]);

  useEffect(() => {
    if (open && inputRef.current) {
      inputRef.current.focus();
    }
  }, [open]);

  const sendMessage = async () => {
    const trimmed = input.trim();
    if (!trimmed || loading) return;

    const userMessage: Message = { role: 'user', content: trimmed };
    const history = [...messages, userMessage];
    setMessages(history);
    setInput('');
    setLoading(true);

    try {
      const res = await fetch('/api/architect/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: trimmed, history: messages }),
      });

      if (!res.ok) throw new Error(`HTTP ${res.status}`);

      const data: { response: string } = await res.json();
      setMessages([...history, { role: 'assistant', content: data.response }]);
    } catch {
      setMessages([
        ...history,
        {
          role: 'assistant',
          content: "Sorry, I couldn't reach the architect. Please try again.",
        },
      ]);
    } finally {
      setLoading(false);
    }
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  return (
    <>
      {/* Floating button */}
      {!open && (
        <button
          className={styles.floatBtn}
          onClick={() => setOpen(true)}
          aria-label="Ask the Architect"
        >
          <svg
            className={styles.chatIcon}
            viewBox="0 0 24 24"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path
              d="M20 2H4C2.9 2 2 2.9 2 4V22L6 18H20C21.1 18 22 17.1 22 16V4C22 2.9 21.1 2 20 2Z"
              fill="currentColor"
            />
          </svg>
          <span className={styles.floatLabel}>Ask the Architect</span>
        </button>
      )}

      {/* Chat panel */}
      {open && (
        <div className={styles.panel} role="dialog" aria-label="GarudaX Architect Chat">
          {/* Header */}
          <div className={styles.header}>
            <div className={styles.headerTitle}>
              <span className={styles.headerDot} />
              GarudaX Architect
            </div>
            <button
              className={styles.closeBtn}
              onClick={() => setOpen(false)}
              aria-label="Close chat"
            >
              ✕
            </button>
          </div>

          {/* Message list */}
          <div className={styles.messages}>
            {messages.length === 0 && (
              <div className={styles.emptyState}>
                Ask me anything about the GarudaX platform architecture.
              </div>
            )}
            {messages.map((msg, i) => (
              <div
                key={i}
                className={msg.role === 'user' ? styles.userBubble : styles.botBubble}
              >
                {msg.role === 'assistant' ? (
                  <div
                    dangerouslySetInnerHTML={{ __html: renderMarkdown(msg.content) }}
                  />
                ) : (
                  <span>{msg.content}</span>
                )}
              </div>
            ))}
            {loading && (
              <div className={styles.botBubble}>
                <div className={styles.typingIndicator}>
                  <span />
                  <span />
                  <span />
                </div>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          {/* Input area */}
          <div className={styles.inputArea}>
            <textarea
              ref={inputRef}
              className={styles.input}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Ask about architecture… (Enter to send, Shift+Enter for newline)"
              rows={2}
              disabled={loading}
            />
            <button
              className={styles.sendBtn}
              onClick={sendMessage}
              disabled={loading || !input.trim()}
              aria-label="Send message"
            >
              <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M2 21L23 12L2 3V10L17 12L2 14V21Z" fill="currentColor" />
              </svg>
            </button>
          </div>
        </div>
      )}
    </>
  );
}
