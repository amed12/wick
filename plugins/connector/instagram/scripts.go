package main

import "fmt"

// navScript navigates without waiting for the page's load event.
//
// Instagram's load event can take longer than the browser plugin's hard 45s
// per-run ceiling (a fresh profile pulls a large bundle plus long-poll
// connections), and a goto that waits for it fails every time. Firing the
// assignment and waiting a fixed beat is both faster and immune to that cap; the
// eval probe that follows reads whatever state the page reached.
func navScript(rawURL string) string {
	return fmt.Sprintf("(() => { location.assign(%q); return {navigating: true}; })()", rawURL)
}

// probeScript reports what the current Instagram page looks like. Used by
// login_status: a logged-out session lands on /accounts/login/ and offers a
// "Log in" link instead of the saved grid.
const probeScript = `(() => {
  const loginLink = document.querySelector('a[href*="accounts/login"]');
  return {
    url: location.href,
    title: document.title,
    logged_in: location.pathname.indexOf('/accounts/login') === -1 && !loginLink,
    at_login: location.pathname.indexOf('/accounts/login') !== -1
  };
})()`

// postScript reads one post page. Instagram puts the caption in og:description
// and the media in og:video / a <video> tag, which is enough to file a card
// without scraping the rendered component tree.
const postScript = `(() => {
  const meta = (p) => { const el = document.querySelector('meta[property="' + p + '"]'); return el ? el.content : null; };
  const canon = document.querySelector('link[rel="canonical"]');
  const video = document.querySelector('video');
  return {
    url: location.href,
    canonical: canon ? canon.href : location.href,
    title: meta('og:title'),
    description: meta('og:description'),
    video_url: video ? (video.src || null) : meta('og:video'),
    at_login: location.pathname.indexOf('/accounts/login') !== -1
  };
})()`

// harvestScript replays the saved-collection GraphQL query inside the page.
//
// Why inside the page: Instagram's web API rejects a plain HTTP client, because
// the __dyn/__hs/__s claims are produced by the SPA bundle at runtime. The
// template argument is the form body the page itself sent (captured once and
// cached); only `variables` is rewritten per call.
func harvestScript(tmpl, collectionID, after string, maxPages, delayMS int) string {
	return fmt.Sprintf(`(async () => {
  const tmpl = %q;
  const collectionId = %q;
  const maxPages = %d;
  const delay = %d;
  let after = %s;
  const items = [];
  let pages = 0;
  let error = null;
  while (pages < maxPages) {
    const params = new URLSearchParams(tmpl);
    const vars = JSON.parse(params.get('variables'));
    vars.after = after;
    vars.collection_id = collectionId;
    vars.first = 100;
    params.set('variables', JSON.stringify(vars));
    const res = await fetch('/api/graphql', {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded' },
      body: params.toString(),
      credentials: 'include'
    });
    const text = await res.text();
    let json;
    try {
      json = JSON.parse(text);
    } catch (e) {
      error = 'non-json response ' + res.status + ': ' + text.slice(0, 120);
      break;
    }
    const mc = json.data && json.data.fetch__MediaCollection;
    if (!mc || !mc.media) {
      error = 'no collection in response';
      break;
    }
    const edges = mc.media.edges || [];
    for (const edge of edges) {
      const n = edge.node || {};
      const owner = (n.user || n.owner || {}).username || '';
      const code = n.code || '';
      if (!code) continue;
      const kind = (n.product_type === 'clips' || n.media_type === 2) ? 'reel' : 'p';
      items.push({
        code: code,
        owner: owner,
        url: 'https://www.instagram.com/' + owner + '/' + kind + '/' + code + '/',
        media_type: n.media_type,
        product_type: n.product_type || null,
        carousel: n.carousel_media_count || null,
        caption: (n.caption && n.caption.text) || ''
      });
    }
    const info = mc.media.page_info || {};
    pages++;
    if (!info.has_next_page || !info.end_cursor) { after = null; break; }
    after = info.end_cursor;
    await new Promise((r) => setTimeout(r, delay));
  }
  return { items: items, pages: pages, next_cursor: after, error: error };
})()`, tmpl, collectionID, maxPages, delayMS, jsLiteral(after))
}

// jsLiteral renders a Go string as a JS expression: null when empty, a quoted
// string otherwise.
func jsLiteral(s string) string {
	if s == "" {
		return "null"
	}
	return fmt.Sprintf("%q", s)
}
