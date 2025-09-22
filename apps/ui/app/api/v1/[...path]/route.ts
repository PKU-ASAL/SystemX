import { NextRequest, NextResponse } from 'next/server';

export async function GET(
    request: NextRequest,
    { params }: { params: { path: string[] } }
) {
    return handleRequest(request, params, 'GET');
}

export async function POST(
    request: NextRequest,
    { params }: { params: { path: string[] } }
) {
    return handleRequest(request, params, 'POST');
}

export async function PUT(
    request: NextRequest,
    { params }: { params: { path: string[] } }
) {
    return handleRequest(request, params, 'PUT');
}

export async function DELETE(
    request: NextRequest,
    { params }: { params: { path: string[] } }
) {
    return handleRequest(request, params, 'DELETE');
}

async function handleRequest(
    request: NextRequest,
    params: { path: string[] },
    method: string
) {
    const managerHost = process.env.MANAGER_HOST || 'sysarmor-manager-1';
    const managerPort = process.env.MANAGER_PORT || '8080';

    const path = params.path.join('/');
    const url = `http://${managerHost}:${managerPort}/api/v1/${path}`;

    // Get query parameters
    const searchParams = request.nextUrl.searchParams;
    const queryString = searchParams.toString();
    const finalUrl = queryString ? `${url}?${queryString}` : url;

    try {
        const headers: HeadersInit = {};

        // Copy relevant headers
        const contentType = request.headers.get('content-type');
        if (contentType) {
            headers['content-type'] = contentType;
        }

        const authorization = request.headers.get('authorization');
        if (authorization) {
            headers['authorization'] = authorization;
        }

        let body: string | undefined;
        if (method !== 'GET' && method !== 'DELETE') {
            body = await request.text();
        }

        const response = await fetch(finalUrl, {
            method,
            headers,
            body,
        });

        const data = await response.text();

        return new NextResponse(data, {
            status: response.status,
            headers: {
                'content-type': response.headers.get('content-type') || 'application/json',
            },
        });
    } catch (error) {
        console.error('Proxy error:', error);
        return NextResponse.json(
            { error: 'Internal Server Error' },
            { status: 500 }
        );
    }
}
