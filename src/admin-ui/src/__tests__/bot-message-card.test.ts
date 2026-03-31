import { describe, it, expect } from 'vitest';
import { parseMessageSegments } from '../components/BotMessageCard';
import { normalizeAction } from '../services/botApi';

// ---------------------------------------------------------------------------
// parseMessageSegments — individual segment types
// ---------------------------------------------------------------------------

describe('parseMessageSegments — success segments', () => {
  it('classifies a ✅ line as success', () => {
    const segs = parseMessageSegments('✅ Order placed successfully');
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('success');
    expect(segs[0].text).toBe('Order placed successfully');
  });

  it('strips the ✅ prefix from the text', () => {
    const segs = parseMessageSegments('✅ Done');
    expect(segs[0].text).toBe('Done');
    expect(segs[0].text).not.toContain('✅');
  });
});

describe('parseMessageSegments — error segments', () => {
  it('classifies a ❌ line as error', () => {
    const segs = parseMessageSegments('❌ Trade failed');
    expect(segs[0].type).toBe('error');
    expect(segs[0].text).toBe('Trade failed');
  });

  it('trims leading whitespace after ❌', () => {
    const segs = parseMessageSegments('❌   Some error');
    expect(segs[0].text).toBe('Some error');
  });
});

describe('parseMessageSegments — warning segments', () => {
  it('classifies a ⚠️ line as warning', () => {
    const segs = parseMessageSegments('⚠️ Margin call approaching');
    expect(segs[0].type).toBe('warning');
  });

  it('strips the ⚠️ emoji from warning text', () => {
    const segs = parseMessageSegments('⚠️ Low balance');
    expect(segs[0].text).not.toMatch(/^⚠/);
    expect(segs[0].text).toBe('Low balance');
  });

  it('also handles bare ⚠ without variation selector', () => {
    const segs = parseMessageSegments('⚠ Caution');
    expect(segs[0].type).toBe('warning');
  });
});

describe('parseMessageSegments — bullet segments', () => {
  it('classifies a • line as bullet', () => {
    const segs = parseMessageSegments('• First item');
    expect(segs[0].type).toBe('bullet');
    expect(segs[0].text).toBe('First item');
  });

  it('classifies a "- " prefixed line as bullet', () => {
    const segs = parseMessageSegments('- Second item');
    expect(segs[0].type).toBe('bullet');
    expect(segs[0].text).toBe('Second item');
  });

  it('does not classify a plain dash without space as bullet', () => {
    const segs = parseMessageSegments('-nodash');
    expect(segs[0].type).toBe('text');
  });
});

describe('parseMessageSegments — heading segments', () => {
  it('classifies **text** (bold markdown) as heading', () => {
    const segs = parseMessageSegments('**Summary**');
    expect(segs[0].type).toBe('heading');
    expect(segs[0].text).toBe('Summary');
  });

  it('classifies "# text" as heading', () => {
    const segs = parseMessageSegments('# Market Status');
    expect(segs[0].type).toBe('heading');
    expect(segs[0].text).toBe('Market Status');
  });

  it('strips ** delimiters from heading text', () => {
    const segs = parseMessageSegments('**Account Info**');
    expect(segs[0].text).toBe('Account Info');
    expect(segs[0].text).not.toContain('**');
  });
});

describe('parseMessageSegments — kv segments', () => {
  it('classifies "Key: Value" pattern as kv', () => {
    const segs = parseMessageSegments('Balance: 10,000 USD');
    expect(segs[0].type).toBe('kv');
    expect(segs[0].key).toBe('Balance');
    expect(segs[0].value).toBe('10,000 USD');
  });

  it('sets both key and value fields', () => {
    const segs = parseMessageSegments('Status: Active');
    expect(segs[0].key).toBe('Status');
    expect(segs[0].value).toBe('Active');
  });

  it('does not match "Key:" without a value', () => {
    // "Key: " has trailing space but no value — should not match kv
    // depends on regex: "Key:  " gives empty match for (.+) — no match → text
    const segs = parseMessageSegments('Key:');
    expect(segs[0].type).toBe('text');
  });
});

describe('parseMessageSegments — text segments', () => {
  it('classifies a plain sentence as text', () => {
    const segs = parseMessageSegments('Here is some information for you.');
    expect(segs[0].type).toBe('text');
    expect(segs[0].text).toBe('Here is some information for you.');
  });

  it('returns empty array for empty string', () => {
    const segs = parseMessageSegments('');
    expect(segs).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// parseMessageSegments — multi-line mixed content
// ---------------------------------------------------------------------------

describe('parseMessageSegments — mixed content', () => {
  it('parses a multi-line reply into correct segment types', () => {
    const reply = [
      '**Account Summary**',
      'Balance: 50,000 USD',
      '• Open Orders: 3',
      '• Filled Today: 7',
      '✅ All systems operational',
    ].join('\n');

    const segs = parseMessageSegments(reply);
    expect(segs).toHaveLength(5);
    expect(segs[0].type).toBe('heading');
    expect(segs[1].type).toBe('kv');
    expect(segs[2].type).toBe('bullet');
    expect(segs[3].type).toBe('bullet');
    expect(segs[4].type).toBe('success');
  });

  it('handles a reply with all seven segment types', () => {
    const reply = [
      '✅ Success line',
      '❌ Error line',
      '⚠️ Warning line',
      '• Bullet line',
      '**Heading line**',
      'Key: Value',
      'Plain text line',
    ].join('\n');

    const segs = parseMessageSegments(reply);
    expect(segs).toHaveLength(7);
    const types = segs.map((s) => s.type);
    expect(types).toEqual(['success', 'error', 'warning', 'bullet', 'heading', 'kv', 'text']);
  });

  it('preserves blank lines as text segments', () => {
    const reply = 'First line\n\nThird line';
    const segs = parseMessageSegments(reply);
    expect(segs).toHaveLength(3);
    expect(segs[1].type).toBe('text');
    expect(segs[1].text).toBe('');
  });
});

// ---------------------------------------------------------------------------
// normalizeAction
// ---------------------------------------------------------------------------

describe('normalizeAction', () => {
  it('maps url field to payload', () => {
    const raw = { id: 'a1', label: 'View Orders', type: 'link', url: '/orders' };
    const action = normalizeAction(raw, 0);
    expect(action.payload).toBe('/orders');
  });

  it('maps target field to payload when url is absent', () => {
    const raw = { id: 'a2', label: 'Go to Dashboard', type: 'link', target: '/dashboard' };
    const action = normalizeAction(raw, 0);
    expect(action.payload).toBe('/dashboard');
  });

  it('prefers payload over url when both are present', () => {
    const raw = { id: 'a3', label: 'Navigate', type: 'link', payload: '/explicit', url: '/ignored' };
    const action = normalizeAction(raw, 0);
    expect(action.payload).toBe('/explicit');
  });

  it('generates an id when none is provided', () => {
    const raw = { label: 'Action', type: 'action', payload: 'do_something' };
    const action = normalizeAction(raw, 5);
    expect(action.id).toBe('act-5');
  });

  it('uses the provided id when present', () => {
    const raw = { id: 'custom-id', label: 'Action', type: 'action', payload: 'x' };
    const action = normalizeAction(raw, 0);
    expect(action.id).toBe('custom-id');
  });

  it('defaults type to "action" when not provided', () => {
    const raw = { id: 'a4', label: 'Do it', payload: 'execute' };
    const action = normalizeAction(raw, 0);
    expect(action.type).toBe('action');
  });

  it('preserves link type from raw', () => {
    const raw = { id: 'a5', label: 'Link', type: 'link', url: '/foo' };
    const action = normalizeAction(raw, 0);
    expect(action.type).toBe('link');
  });

  it('sets empty string payload when no payload/url/target', () => {
    const raw = { id: 'a6', label: 'Empty', type: 'action' };
    const action = normalizeAction(raw, 0);
    expect(action.payload).toBe('');
  });
});
