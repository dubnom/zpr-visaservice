// Optional diagnostic preload: observes browser events without changing CSP or editor behavior.
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
const { chromium } = await import(pathToFileURL(resolve(process.cwd(), 'node_modules/playwright/index.mjs')));
const launch = chromium.launch.bind(chromium);
chromium.launch = async (...args) => {
  const browser = await launch(...args);
  console.log('CSP_DIAGNOSTIC ' + JSON.stringify({ browserVersion: browser.version() }));
  const newContext = browser.newContext.bind(browser);
  browser.newContext = async (...contextArgs) => {
    const context = await newContext(...contextArgs);
    context.on('page', async page => {
      const session = await context.newCDPSession(page);
      session.on('Audits.issueAdded', async ({ issue }) => {
        if (issue.code !== 'ContentSecurityPolicyIssue') return;
        const details = issue.details.contentSecurityPolicyIssueDetails;
        let node;
        if (details.violatingNodeId) {
          try { node = (await session.send('DOM.describeNode', { backendNodeId: details.violatingNodeId })).node; }
          catch (error) { node = { unavailable: error.message }; }
        }
        console.log('CSP_DIAGNOSTIC ' + JSON.stringify({ audit: details, node }));
      });
      await session.send('Audits.enable');
      page.on('console', message => {
        if (message.text().startsWith('CSP_DIAGNOSTIC ') || message.text().includes('Applying inline style')) console.log(message.text());
      });
    });
    await context.addInitScript(observeInput => {
      const mutations = [];
      const remember = value => {
        mutations.push(value);
        if (mutations.length > 12) mutations.shift();
      };
      const observer = new MutationObserver(records => {
        for (const record of records) {
          if (record.type === 'attributes' && record.attributeName === 'style') {
            remember({ kind: 'attribute', oldStyle: record.oldValue, html: record.target.outerHTML.slice(0, 1200) });
            if (observeInput && record.target.classList.contains('cm-line') && (record.oldValue || record.target.getAttribute('style'))) {
              console.log('CSP_DIAGNOSTIC ' + JSON.stringify({ lineStyleMutation: { oldStyle: record.oldValue,
                currentStyle: record.target.getAttribute('style'), html: record.target.outerHTML.slice(0, 1200) } }));
            }
          }
          for (const node of record.addedNodes) {
            if (node.nodeType === 1 && (node.hasAttribute('style') || node.querySelector('[style]'))) {
              remember({ kind: 'insert', html: node.outerHTML.slice(0, 1200) });
            }
          }
        }
      });
      const options = { subtree: true, childList: true, attributes: true, attributeOldValue: true, attributeFilter: ['style'] };
      observer.observe(document, options);
      const attachShadow = Element.prototype.attachShadow;
      Element.prototype.attachShadow = function (...args) {
        const root = attachShadow.apply(this, args);
        observer.observe(root, options);
        return root;
      };
      for (const type of ['beforeinput', 'input']) document.addEventListener(type, event => {
        const host = document.activeElement;
        const root = host?.shadowRoot;
        remember({ kind: type, inputType: event.inputType, html: root?.querySelector('.cm-content')?.innerHTML.slice(0, 2000) });
        if (observeInput && type === 'beforeinput') {
          const selection = root?.getSelection?.();
          console.log('CSP_DIAGNOSTIC ' + JSON.stringify({ input: { inputType: event.inputType, isTrusted: event.isTrusted,
            anchor: selection?.anchorNode?.parentElement?.outerHTML.slice(0, 800),
            focus: selection?.focusNode?.parentElement?.outerHTML.slice(0, 800),
            matchingBrackets: [...(root?.querySelectorAll('.cm-matchingBracket') || [])].map(node => ({ html: node.outerHTML,
              color: getComputedStyle(node).color, backgroundColor: getComputedStyle(node).backgroundColor })) } }));
        }
      }, true);
      document.addEventListener('securitypolicyviolation', event => {
        console.log('CSP_DIAGNOSTIC ' + JSON.stringify({
          directive: event.effectiveDirective, blockedURI: event.blockedURI,
          sourceFile: event.sourceFile, lineNumber: event.lineNumber,
          columnNumber: event.columnNumber, sample: event.sample,
          activeElement: document.activeElement?.outerHTML.slice(0, 2000),
          mutations: [...mutations],
        }));
      });
    }, process.env.CSP_OBSERVE_INPUT === '1');
    return context;
  };
  return browser;
};
