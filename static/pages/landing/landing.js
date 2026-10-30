(function () {
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const nodes = Array.from(document.querySelectorAll('.word-node'));

    function flashNode(node, duration = 260) {
        if (!node || node.classList.contains('node-glow')) return;
        node.classList.add('node-glow');
        setTimeout(() => {
            node.classList.remove('node-glow');
        }, duration);
    }

    if (!reduceMotion && nodes.length) {
        function triggerRandomGlow() {
            const node = nodes[Math.floor(Math.random() * nodes.length)];
            flashNode(node, 220);
            const nextDelay = 700 + Math.random() * 1300;
            setTimeout(triggerRandomGlow, nextDelay);
        }

        setTimeout(triggerRandomGlow, 800);

        let lastMove = 0;
        window.addEventListener('mousemove', (e) => {
            const now = performance.now();
            if (now - lastMove < 70) return;
            lastMove = now;

            for (let i = 0; i < nodes.length; i++) {
                const rect = nodes[i].getBoundingClientRect();
                if (rect.width === 0) continue;
                const cx = rect.left + rect.width / 2;
                const cy = rect.top + rect.height / 2;
                const dist = Math.hypot(e.clientX - cx, e.clientY - cy);
                if (dist < 65) {
                    flashNode(nodes[i], 380);
                }
            }
        }, { passive: true });
    }

    const icons = document.querySelectorAll('.preview-icon');
    icons.forEach((icon) => {
        const card = icon.closest('p');
        if (card) {
            card.classList.add('feature-card-anim');
        }
    });

    if (!reduceMotion && 'IntersectionObserver' in window) {
        const revealTargets = document.querySelectorAll(
            'div.text-zinc-400.mt-12, div.mt-12.text-xl > p, p.text-cyan-500.font-bold.text-3xl'
        );

        revealTargets.forEach((el, idx) => {
            el.classList.add('reveal-item');
            el.style.transitionDelay = `${Math.min(idx * 90, 270)}ms`;
        });

        const observer = new IntersectionObserver(
            (entries) => {
                entries.forEach((entry) => {
                    if (entry.isIntersecting) {
                        entry.target.classList.add('is-visible');
                        observer.unobserve(entry.target);
                    }
                });
            },
            { threshold: 0.15 }
        );

        revealTargets.forEach((el) => observer.observe(el));
    }
})();



