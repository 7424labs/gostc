console.log('gostc Static File Server - Example Application');

document.addEventListener('DOMContentLoaded', function() {
    console.log('DOM loaded successfully');

    fetch('/data.json')
        .then(response => response.json())
        .then(data => {
            console.log('Fetched data:', data);

            const container = document.querySelector('.container');
            const info = document.createElement('div');
            info.style.marginTop = '2rem';
            info.style.padding = '1rem';
            info.style.background = '#f8f9fa';
            info.style.borderRadius = '5px';

            info.innerHTML = `
                <h3>Server Info (from data.json)</h3>
                <p>Name: ${data.name}</p>
                <p>Version: ${data.version}</p>
                <p>Description: ${data.description}</p>
            `;

            container.appendChild(info);
        })
        .catch(error => console.error('Error fetching data:', error));

    checkCompression();
});

function checkCompression() {
    const req = new XMLHttpRequest();
    req.open('HEAD', '/style.css', true);
    req.onreadystatechange = function() {
        if (this.readyState === this.DONE) {
            const encoding = this.getResponseHeader('Content-Encoding');
            if (encoding) {
                console.log(`CSS file served with ${encoding} compression`);

                const compressionInfo = document.createElement('p');
                compressionInfo.style.marginTop = '1rem';
                compressionInfo.style.padding = '0.5rem';
                compressionInfo.style.background = '#d4edda';
                compressionInfo.style.color = '#155724';
                compressionInfo.style.borderRadius = '3px';
                compressionInfo.textContent = `✓ Compression active: ${encoding}`;

                document.querySelector('.container').appendChild(compressionInfo);
            }
        }
    };
    req.send();
}