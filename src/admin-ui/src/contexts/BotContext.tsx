import React, { createContext, useContext, useReducer, useCallback, useEffect } from 'react';
import { sendBotMessage, getBotSuggestions, Suggestion, Action } from '../services/botApi';

export interface BotMessage {
  id: string;
  role: 'user' | 'bot';
  content: string;
  timestamp: number;
  actions?: Action[];
}

export interface BotState {
  isOpen: boolean;
  messages: BotMessage[];
  unreadCount: number;
  isTyping: boolean;
  suggestions: Suggestion[];
  showTicketForm: boolean;
  ticketCategory: string;
}

export type BotAction =
  | { type: 'TOGGLE_PANEL' }
  | { type: 'CLOSE_PANEL' }
  | { type: 'ADD_USER_MESSAGE'; payload: { id: string; content: string; timestamp: number } }
  | { type: 'ADD_BOT_MESSAGE'; payload: { id: string; content: string; timestamp: number; actions?: Action[] } }
  | { type: 'SET_TYPING'; payload: boolean }
  | { type: 'CLEAR_UNREAD' }
  | { type: 'SET_SUGGESTIONS'; payload: Suggestion[] }
  | { type: 'SHOW_TICKET_FORM'; payload: { category: string } }
  | { type: 'HIDE_TICKET_FORM' };

export const initialBotState: BotState = {
  isOpen: false,
  messages: [],
  unreadCount: 0,
  isTyping: false,
  suggestions: [],
  showTicketForm: false,
  ticketCategory: 'support',
};

export function botReducer(state: BotState, action: BotAction): BotState {
  switch (action.type) {
    case 'TOGGLE_PANEL':
      return {
        ...state,
        isOpen: !state.isOpen,
        unreadCount: !state.isOpen ? state.unreadCount : 0,
      };
    case 'CLOSE_PANEL':
      return { ...state, isOpen: false };
    case 'ADD_USER_MESSAGE':
      return {
        ...state,
        messages: [
          ...state.messages,
          {
            id: action.payload.id,
            role: 'user',
            content: action.payload.content,
            timestamp: action.payload.timestamp,
          },
        ],
      };
    case 'ADD_BOT_MESSAGE':
      return {
        ...state,
        messages: [
          ...state.messages,
          {
            id: action.payload.id,
            role: 'bot',
            content: action.payload.content,
            timestamp: action.payload.timestamp,
            actions: action.payload.actions,
          },
        ],
        unreadCount: state.isOpen ? state.unreadCount : state.unreadCount + 1,
      };
    case 'SET_TYPING':
      return { ...state, isTyping: action.payload };
    case 'CLEAR_UNREAD':
      return { ...state, unreadCount: 0 };
    case 'SET_SUGGESTIONS':
      return { ...state, suggestions: action.payload };
    case 'SHOW_TICKET_FORM':
      return { ...state, showTicketForm: true, ticketCategory: action.payload.category };
    case 'HIDE_TICKET_FORM':
      return { ...state, showTicketForm: false };
    default:
      return state;
  }
}

export function generateMessageId(): string {
  return `msg-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

export function formatMessageTime(timestamp: number): string {
  const date = new Date(timestamp);
  const hours = date.getHours().toString().padStart(2, '0');
  const minutes = date.getMinutes().toString().padStart(2, '0');
  return `${hours}:${minutes}`;
}

export function filterSuggestionsByQuery(suggestions: Suggestion[], query: string): Suggestion[] {
  if (!query) return suggestions;
  const lower = query.toLowerCase();
  return suggestions.filter(
    (s) => s.label.toLowerCase().includes(lower) || s.prompt.toLowerCase().includes(lower),
  );
}

interface BotContextValue {
  state: BotState;
  sendMessage: (text: string) => void;
  togglePanel: () => void;
  closePanel: () => void;
  clearUnread: () => void;
  setSuggestions: (suggestions: Suggestion[]) => void;
  showTicketForm: (category: string) => void;
  hideTicketForm: () => void;
  loadSuggestions: (page: string) => void;
}

const BotContext = createContext<BotContextValue | null>(null);

export function BotProvider({
  children,
  currentPage,
}: {
  children: React.ReactNode;
  currentPage?: string;
}) {
  const [state, dispatch] = useReducer(botReducer, initialBotState);

  const sendMessage = useCallback((text: string) => {
    const userMsgId = generateMessageId();
    const now = Date.now();

    dispatch({
      type: 'ADD_USER_MESSAGE',
      payload: { id: userMsgId, content: text, timestamp: now },
    });
    dispatch({ type: 'SET_TYPING', payload: true });

    sendBotMessage(text, { page: currentPage })
      .then((response) => {
        const botMsgId = generateMessageId();
        dispatch({
          type: 'ADD_BOT_MESSAGE',
          payload: {
            id: botMsgId,
            content: response.reply,
            timestamp: Date.now(),
            actions: response.actions,
          },
        });
        if (response.suggestions) {
          dispatch({ type: 'SET_SUGGESTIONS', payload: response.suggestions });
        }
      })
      .catch(() => {
        const errorMsgId = generateMessageId();
        dispatch({
          type: 'ADD_BOT_MESSAGE',
          payload: {
            id: errorMsgId,
            content: 'Sorry, I encountered an error. Please try again.',
            timestamp: Date.now(),
          },
        });
      })
      .finally(() => {
        dispatch({ type: 'SET_TYPING', payload: false });
      });
  }, [currentPage]);

  const togglePanel = useCallback(() => {
    dispatch({ type: 'TOGGLE_PANEL' });
  }, []);

  const closePanel = useCallback(() => {
    dispatch({ type: 'CLOSE_PANEL' });
  }, []);

  const clearUnread = useCallback(() => {
    dispatch({ type: 'CLEAR_UNREAD' });
  }, []);

  const setSuggestions = useCallback((suggestions: Suggestion[]) => {
    dispatch({ type: 'SET_SUGGESTIONS', payload: suggestions });
  }, []);

  const showTicketFormAction = useCallback((category: string) => {
    dispatch({ type: 'SHOW_TICKET_FORM', payload: { category } });
  }, []);

  const hideTicketForm = useCallback(() => {
    dispatch({ type: 'HIDE_TICKET_FORM' });
  }, []);

  const loadSuggestions = useCallback((_page: string) => {
    // Bot suggestions endpoint not available in current deployment — skip to avoid console noise
    dispatch({ type: 'SET_SUGGESTIONS', payload: [] });
  }, []);

  useEffect(() => {
    if (currentPage) {
      loadSuggestions(currentPage);
    }
  }, [currentPage, loadSuggestions]);

  return (
    <BotContext.Provider
      value={{
        state,
        sendMessage,
        togglePanel,
        closePanel,
        clearUnread,
        setSuggestions,
        showTicketForm: showTicketFormAction,
        hideTicketForm,
        loadSuggestions,
      }}
    >
      {children}
    </BotContext.Provider>
  );
}

export function useBot(): BotContextValue {
  const ctx = useContext(BotContext);
  if (!ctx) throw new Error('useBot must be used within BotProvider');
  return ctx;
}
