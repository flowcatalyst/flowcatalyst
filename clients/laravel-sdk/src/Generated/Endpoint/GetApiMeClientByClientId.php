<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiMeClientByClientId extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $clientId;
    /**
     * @param string $clientId
     */
    public function __construct(string $clientId)
    {
        $this->clientId = $clientId;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{clientId}'], [$this->clientId], '/api/me/clients/{clientId}');
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
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse200', 'json');
        }
        if (is_null($contentType) === false && (401 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdUnauthorizedException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse401', 'json'), $response);
        }
        if (is_null($contentType) === false && (403 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdForbiddenException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse403', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}