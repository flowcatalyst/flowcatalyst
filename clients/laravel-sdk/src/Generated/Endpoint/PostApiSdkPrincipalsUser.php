<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiSdkPrincipalsUser extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostBody $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostBody $requestBody = null)
    {
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/api/sdk/principals/user';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostBody) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsUserBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsUserConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse201', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsUserBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsUserConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}