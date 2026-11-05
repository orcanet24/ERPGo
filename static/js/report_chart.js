// report_chart.js — Dibujo del gráfico de los informes personalizados.
//
// El gráfico se alimenta de las MISMAS filas del informe: el servidor manda
// `chart_data` ({type,title,labels,datasets:[{label,data}]}) y esta función lo
// pinta con Chart.js 2.5 (la versión que sirve layout.html).
//
// Se comparte entre el modal de creación asistida y la pantalla de ejecución
// para que ambas pantallas dibujen exactamente igual. Antes cada pantalla
// tenía su copia del render y ya habían divergido.
(function () {
    'use strict';

    function tone(name) {
        var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
        return v || '';
    }

    // La paleta sale del TEMA, no de una lista fija: las porciones de una tarta
    // usan el mismo color que la serie equivalente del resto del ERP y cambian
    // al cambiar de tema (incluido el modo oscuro).
    function palette() {
        var series = [
            '--erp-series-1', '--erp-series-2', '--erp-series-3', '--erp-series-4',
            '--erp-series-5', '--erp-series-6', '--erp-series-7', '--erp-series-8'
        ];
        var out = [];
        for (var i = 0; i < series.length; i++) {
            var c = tone(series[i]);
            if (c) { out.push(c); }
        }
        return out.length ? out : ['#1a4f68', '#3f8f7d', '#b3801f', '#7d6a99',
                                   '#b0413e', '#2d7dd2', '#c2683f', '#4f8a45'];
    }

    // render dibuja (o redibuja) el gráfico en el canvas indicado.
    // Devuelve false si no hay canvas o el dato no sirve, para que el caller
    // pueda esconder el contenedor en vez de dejar un lienzo vacío.
    function render(canvasId, data) {
        var canvas = document.getElementById(canvasId);
        if (!canvas || !data || !data.labels || !data.datasets || !data.datasets.length) {
            return false;
        }
        var pal = palette();
        var isPie = data.type === 'pie' || data.type === 'doughnut';
        var datasets = [];
        for (var i = 0; i < data.datasets.length; i++) {
            var ds = data.datasets[i];
            if (isPie) {
                var colors = [];
                for (var j = 0; j < data.labels.length; j++) {
                    colors.push(pal[j % pal.length]);
                }
                datasets.push({ label: ds.label, data: ds.data, backgroundColor: colors });
            } else {
                var c = pal[i % pal.length];
                datasets.push({
                    label: ds.label,
                    data: ds.data,
                    borderColor: c,
                    backgroundColor: c + '22',
                    fill: data.type === 'line'
                });
            }
        }

        // Chart.js 2.5: la configuración es options.legend / options.title /
        // options.scales.yAxes, NO el objeto `plugins` de Chart.js 3+.
        var options = {
            responsive: true,
            maintainAspectRatio: true,
            legend: { display: data.show_legend !== false },
            title: { display: !!data.title, text: data.title || '' }
        };
        if (!data.type || data.type === 'bar' || data.type === 'line') {
            options.scales = {
                yAxes: [{ ticks: { beginAtZero: true } }],
                xAxes: [{ gridLines: { display: false } }]
            };
        }

        if (canvas.__erpChart) { canvas.__erpChart.destroy(); }
        canvas.__erpChart = new Chart(canvas.getContext('2d'), {
            type: data.type || 'line',
            data: { labels: data.labels, datasets: datasets },
            options: options
        });
        return true;
    }

    function clear(canvasId) {
        var canvas = document.getElementById(canvasId);
        if (canvas && canvas.__erpChart) {
            canvas.__erpChart.destroy();
            canvas.__erpChart = null;
        }
    }

    window.ERPGoReportChart = { render: render, clear: clear };
})();
