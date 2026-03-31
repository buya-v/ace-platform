import React, { useState, useRef, useEffect } from 'react';
import { useBot } from '../contexts/BotContext';
import { formatMessageTime } from '../contexts/BotContext';
import { BotTicketForm } from './BotTicketForm';
import { BotMessageCard } from './BotMessageCard';
import styles from './BotChatPanel.module.css';

export function BotChatPanel() {
  const { state, sendMessage, closePanel, clearUnread, showTicketForm } = useBot();
  const { isOpen, messages, isTyping, suggestions, showTicketForm: showForm } = state;
  const [input, setInput] = useState('');
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isOpen) {
      clearUnread();
    }
  }, [isOpen, clearUnread]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, isTyping]);

  if (!isOpen) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = input.trim();
    if (!trimmed) return;
    sendMessage(trimmed);
    setInput('');
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  const handleSuggestionClick = (prompt: string) => {
    sendMessage(prompt);
  };

  return (
    <div className={styles.panel} role="dialog" aria-label="GarudaX Bot Chat">
      {/* Header */}
      <div className={styles.header}>
        <span className={styles.headerTitle}>
          <span className={styles.headerDot} />
          GarudaX Bot
        </span>
        <button
          className={styles.closeBtn}
          onClick={closePanel}
          aria-label="Close chat"
          type="button"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path
              d="M4 4L12 12M12 4L4 12"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
            />
          </svg>
        </button>
      </div>

      {/* Messages */}
      <div className={styles.messages}>
        {messages.length === 0 && (
          <div className={styles.botMessage} style={{ alignSelf: 'flex-start' }}>
            <div className={styles.messageBubble}>
              Hi! I am the GarudaX Bot. How can I help you today?
            </div>
          </div>
        )}
        {messages.map((msg) => (
          <div
            key={msg.id}
            className={`${styles.messageBubble} ${
              msg.role === 'user' ? styles.userMessage : styles.botMessage
            }`}
          >
            {msg.role === 'bot' ? (
              <BotMessageCard reply={msg.content} actions={msg.actions} />
            ) : (
              msg.content
            )}
            <div className={styles.messageTime}>{formatMessageTime(msg.timestamp)}</div>
            {msg.role === 'user' && msg.actions && msg.actions.length > 0 && (
              <div className={styles.actionChips}>
                {msg.actions.map((action) => (
                  <button
                    key={action.id}
                    className={styles.actionChip}
                    onClick={() => sendMessage(action.label)}
                    type="button"
                  >
                    {action.label}
                  </button>
                ))}
              </div>
            )}
          </div>
        ))}
        {isTyping && (
          <div className={styles.typingIndicator}>
            <span className={styles.typingDot} />
            <span className={styles.typingDot} />
            <span className={styles.typingDot} />
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Ticket Form (inline) */}
      {showForm && <BotTicketForm />}

      {/* Suggestions */}
      {suggestions.length > 0 && !showForm && (
        <div className={styles.suggestionsRow}>
          {suggestions.map((s) => (
            <button
              key={s.id}
              className={styles.suggestionChip}
              onClick={() => handleSuggestionClick(s.prompt)}
              type="button"
            >
              {s.label}
            </button>
          ))}
        </div>
      )}

      {/* Input */}
      {!showForm && (
        <form className={styles.inputArea} onSubmit={handleSubmit}>
          <input
            className={styles.input}
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Type a message..."
            aria-label="Chat message input"
          />
          <button
            className={styles.sendBtn}
            type="submit"
            disabled={!input.trim()}
            aria-label="Send message"
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
              <path
                d="M2 8L14 2L8 14L7 9L2 8Z"
                fill="currentColor"
                stroke="currentColor"
                strokeWidth="1"
                strokeLinejoin="round"
              />
            </svg>
          </button>
        </form>
      )}

      {/* Shortcuts */}
      {!showForm && (
        <div className={styles.shortcuts}>
          <button
            className={styles.shortcutBtn}
            onClick={() => showTicketForm('bug_report')}
            type="button"
          >
            Report Bug
          </button>
          <button
            className={styles.shortcutBtn}
            onClick={() => showTicketForm('feature_request')}
            type="button"
          >
            Request Feature
          </button>
        </div>
      )}
    </div>
  );
}
