(function () {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

    const nodes = Array.from(document.querySelectorAll('.word-node'));
    if (!nodes.length) return;

    function triggerRandomGlow() {
        const node = nodes[Math.floor(Math.random() * nodes.length)];
        if (!node.classList.contains('node-glow')) {
            node.classList.add('node-glow');
            setTimeout(() => {
                node.classList.remove('node-glow');
            }, 220);
        }

        const nextDelay = 700 + Math.random() * 1300;
        setTimeout(triggerRandomGlow, nextDelay);
    }

    setTimeout(triggerRandomGlow, 800);
})();
