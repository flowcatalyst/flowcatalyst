// <fc-diagram>: renders the Mermaid source in its <pre> as an SVG, in
// Mermaid's strict security level. The Mermaid bundle (data-bundle, a
// same-origin URL) is loaded once, on the first diagram, so pages without
// diagrams never fetch it. Without script the source stays readable.
let loading;

function loadMermaid(src) {
  if (globalThis.mermaid) return Promise.resolve(globalThis.mermaid);
  loading ??= new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = () => resolve(globalThis.mermaid);
    s.onerror = () => reject(new Error("mermaid bundle did not load"));
    document.head.append(s);
  });
  return loading;
}

let seq = 0;

class FCDiagram extends HTMLElement {
  connectedCallback() {
    const view = this.querySelector(".fc-diagram-view");
    const source = this.querySelector("pre")?.textContent ?? "";
    if (!view || !source.trim() || view.dataset.drawn) return;
    view.dataset.drawn = "";
    view.textContent = "Rendering diagram...";
    view.hidden = false;
    loadMermaid(this.dataset.bundle)
      .then(async (mermaid) => {
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: "strict",
          suppressErrorRendering: true,
          theme: document.documentElement.classList.contains("dark") ? "dark" : "default",
        });
        const { svg } = await mermaid.render("fc-diagram-" + ++seq, source);
        view.innerHTML = svg;
      })
      .catch(() => {
        document.getElementById("dfc-diagram-" + seq)?.remove(); // Mermaid's scratch node after a failed render
        view.hidden = true;
        this.querySelector("details")?.setAttribute("open", "");
        this.dataset.failed = "";
      });
  }
}

if (!customElements.get("fc-diagram")) customElements.define("fc-diagram", FCDiagram);
