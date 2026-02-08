<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkRevokeClientAccess extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $clientId;
    protected $id;
    /**
     * @param string $clientId
     * @param string $id
     */
    public function __construct(string $clientId, string $id)
    {
        $this->clientId = $clientId;
        $this->id = $id;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'DELETE';
    }
    public function getUri(): string
    {
        return str_replace(['{clientId}', '{id}'], [$this->clientId, $this->id], '/api/sdk/principals/{id}/clients/{clientId}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessForbiddenException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (204 === $status) {
            return null;
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}