<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiClientsByIdApplication extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * Returns applications enabled for the specified client.
     * @param string $id
     */
    public function __construct(string $id)
    {
        $this->id = $id;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/clients/{id}/applications');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApplicationListResponse2
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApplicationListResponse2', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationBadRequestException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationForbiddenException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationNotFoundException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}